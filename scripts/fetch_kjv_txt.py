#!/usr/bin/env python3
"""
KJV Bible Fetcher from OpenBible.com
Fetches KJV from https://openbible.com/textfiles/kjv.txt
"""

import requests
import json
import sys
import re
from typing import List, Dict, Optional
from datetime import datetime

# English book names with book numbers and chapter counts
BOOKS = [
    {"number": 1, "name": "Genesis", "abbr": "Gen", "chapters": 50},
    {"number": 2, "name": "Exodus", "abbr": "Exod", "chapters": 40},
    {"number": 3, "name": "Leviticus", "abbr": "Lev", "chapters": 27},
    {"number": 4, "name": "Numbers", "abbr": "Num", "chapters": 36},
    {"number": 5, "name": "Deuteronomy", "abbr": "Deut", "chapters": 34},
    {"number": 6, "name": "Joshua", "abbr": "Josh", "chapters": 24},
    {"number": 7, "name": "Judges", "abbr": "Judg", "chapters": 21},
    {"number": 8, "name": "Ruth", "abbr": "Ruth", "chapters": 4},
    {"number": 9, "name": "1 Samuel", "abbr": "1Sam", "chapters": 31},
    {"number": 10, "name": "2 Samuel", "abbr": "2Sam", "chapters": 24},
    {"number": 11, "name": "1 Kings", "abbr": "1Kgs", "chapters": 22},
    {"number": 12, "name": "2 Kings", "abbr": "2Kgs", "chapters": 25},
    {"number": 13, "name": "1 Chronicles", "abbr": "1Chr", "chapters": 29},
    {"number": 14, "name": "2 Chronicles", "abbr": "2Chr", "chapters": 36},
    {"number": 15, "name": "Ezra", "abbr": "Ezra", "chapters": 10},
    {"number": 16, "name": "Nehemiah", "abbr": "Neh", "chapters": 13},
    {"number": 17, "name": "Esther", "abbr": "Esth", "chapters": 10},
    {"number": 18, "name": "Job", "abbr": "Job", "chapters": 42},
    {"number": 19, "name": "Psalms", "abbr": "Ps", "chapters": 150},
    {"number": 20, "name": "Proverbs", "abbr": "Prov", "chapters": 31},
    {"number": 21, "name": "Ecclesiastes", "abbr": "Eccl", "chapters": 12},
    {"number": 22, "name": "Song of Solomon", "abbr": "Song", "chapters": 8},
    {"number": 23, "name": "Isaiah", "abbr": "Isa", "chapters": 66},
    {"number": 24, "name": "Jeremiah", "abbr": "Jer", "chapters": 52},
    {"number": 25, "name": "Lamentations", "abbr": "Lam", "chapters": 5},
    {"number": 26, "name": "Ezekiel", "abbr": "Ezek", "chapters": 48},
    {"number": 27, "name": "Daniel", "abbr": "Dan", "chapters": 12},
    {"number": 28, "name": "Hosea", "abbr": "Hos", "chapters": 14},
    {"number": 29, "name": "Joel", "abbr": "Joel", "chapters": 3},
    {"number": 30, "name": "Amos", "abbr": "Amos", "chapters": 9},
    {"number": 31, "name": "Obadiah", "abbr": "Obad", "chapters": 1},
    {"number": 32, "name": "Jonah", "abbr": "Jonah", "chapters": 4},
    {"number": 33, "name": "Micah", "abbr": "Mic", "chapters": 7},
    {"number": 34, "name": "Nahum", "abbr": "Nah", "chapters": 3},
    {"number": 35, "name": "Habakkuk", "abbr": "Hab", "chapters": 3},
    {"number": 36, "name": "Zephaniah", "abbr": "Zeph", "chapters": 3},
    {"number": 37, "name": "Haggai", "abbr": "Hag", "chapters": 2},
    {"number": 38, "name": "Zechariah", "abbr": "Zech", "chapters": 14},
    {"number": 39, "name": "Malachi", "abbr": "Mal", "chapters": 4},
    {"number": 40, "name": "Matthew", "abbr": "Matt", "chapters": 28},
    {"number": 41, "name": "Mark", "abbr": "Mark", "chapters": 16},
    {"number": 42, "name": "Luke", "abbr": "Luke", "chapters": 24},
    {"number": 43, "name": "John", "abbr": "John", "chapters": 21},
    {"number": 44, "name": "Acts", "abbr": "Acts", "chapters": 28},
    {"number": 45, "name": "Romans", "abbr": "Rom", "chapters": 16},
    {"number": 46, "name": "1 Corinthians", "abbr": "1Cor", "chapters": 16},
    {"number": 47, "name": "2 Corinthians", "abbr": "2Cor", "chapters": 13},
    {"number": 48, "name": "Galatians", "abbr": "Gal", "chapters": 6},
    {"number": 49, "name": "Ephesians", "abbr": "Eph", "chapters": 6},
    {"number": 50, "name": "Philippians", "abbr": "Phil", "chapters": 4},
    {"number": 51, "name": "Colossians", "abbr": "Col", "chapters": 4},
    {"number": 52, "name": "1 Thessalonians", "abbr": "1Thess", "chapters": 5},
    {"number": 53, "name": "2 Thessalonians", "abbr": "2Thess", "chapters": 3},
    {"number": 54, "name": "1 Timothy", "abbr": "1Tim", "chapters": 6},
    {"number": 55, "name": "2 Timothy", "abbr": "2Tim", "chapters": 4},
    {"number": 56, "name": "Titus", "abbr": "Titus", "chapters": 3},
    {"number": 57, "name": "Philemon", "abbr": "Phlm", "chapters": 1},
    {"number": 58, "name": "Hebrews", "abbr": "Heb", "chapters": 13},
    {"number": 59, "name": "James", "abbr": "Jas", "chapters": 5},
    {"number": 60, "name": "1 Peter", "abbr": "1Pet", "chapters": 5},
    {"number": 61, "name": "2 Peter", "abbr": "2Pet", "chapters": 3},
    {"number": 62, "name": "1 John", "abbr": "1John", "chapters": 5},
    {"number": 63, "name": "2 John", "abbr": "2John", "chapters": 1},
    {"number": 64, "name": "3 John", "abbr": "3John", "chapters": 1},
    {"number": 65, "name": "Jude", "abbr": "Jude", "chapters": 1},
    {"number": 66, "name": "Revelation", "abbr": "Rev", "chapters": 22},
]

# Map book names from text file to our book numbers
# The text file uses formats like "Genesis", "1 Samuel", "1 Kings", "Matthew", etc.
BOOK_NAME_MAP = {}
for book in BOOKS:
    name = book["name"]
    # Add exact match (case insensitive)
    BOOK_NAME_MAP[name.lower()] = book["number"]
    # Add without spaces
    BOOK_NAME_MAP[name.replace(" ", "").lower()] = book["number"]
    
    # Handle numbered books (1 Samuel, 2 Samuel, etc.)
    if " " in name and name[0].isdigit():
        num_part = name.split()[0]
        name_part = " ".join(name.split()[1:])
        # Format: "1 Samuel" (most common in text file)
        BOOK_NAME_MAP[f"{num_part} {name_part}".lower()] = book["number"]
        # Also add without space: "1Samuel"
        BOOK_NAME_MAP[f"{num_part}{name_part}".lower()] = book["number"]

# Special cases for text file format differences
BOOK_NAME_MAP["psalm"] = 19  # Text file uses "Psalm" instead of "Psalms"
BOOK_NAME_MAP["song of songs"] = 22  # Alternative name for Song of Solomon
BOOK_NAME_MAP["song of solomon"] = 22

KJV_URL = "https://openbible.com/textfiles/kjv.txt"

# Global log file handle
_log_file: Optional[object] = None


def log_print(*args, **kwargs):
    """Print to both console and log file"""
    print(*args, **kwargs)
    if _log_file:
        log_kwargs = kwargs.copy()
        log_kwargs.pop('end', None)
        log_kwargs.pop('flush', None)
        log_kwargs['file'] = _log_file
        print(*args, **log_kwargs)
        _log_file.flush()


def init_log_file(log_path: Optional[str] = None):
    """Initialize log file"""
    global _log_file
    if log_path:
        try:
            _log_file = open(log_path, 'a', encoding='utf-8')
            log_print(f"\n{'='*60}")
            log_print(f"Log started at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
            log_print(f"{'='*60}\n")
            return True
        except Exception as e:
            print(f"WARNING: Failed to open log file {log_path}: {e}", file=sys.stderr)
            _log_file = None
            return False
    return False


def close_log_file():
    """Close log file"""
    global _log_file
    if _log_file:
        try:
            log_print(f"\n{'='*60}")
            log_print(f"Log ended at {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
            log_print(f"{'='*60}\n")
            _log_file.close()
        except:
            pass
        finally:
            _log_file = None


def parse_book_name(text: str) -> Optional[int]:
    """Parse book name from text and return book number"""
    # Format: "Genesis 1:1" or "1 Samuel 1:1"
    # Extract book name part (everything before the first number)
    match = re.match(r'^([^0-9]+?)\s+(\d+):(\d+)', text)
    if not match:
        return None
    
    book_name = match.group(1).strip()
    return BOOK_NAME_MAP.get(book_name.lower())


def fetch_kjv_text() -> str:
    """Fetch KJV text file from OpenBible.com"""
    log_print(f"Fetching KJV text from {KJV_URL}...")
    try:
        response = requests.get(KJV_URL, timeout=60)
        response.raise_for_status()
        # Handle BOM and encoding
        text = response.content.decode('utf-8-sig')  # utf-8-sig removes BOM
        log_print(f"✅ Successfully downloaded {len(text)} characters")
        return text
    except Exception as e:
        log_print(f"ERROR: Failed to fetch KJV text: {e}", file=sys.stderr)
        sys.exit(1)


def parse_kjv_text(text: str) -> Dict:
    """Parse KJV text file into structured JSON format"""
    log_print("Parsing KJV text...")
    
    output_data = {
        "version": {
            "code": "KJV",
            "name": "King James Version"
        },
        "books": []
    }
    
    # Initialize book structures
    books_dict = {}
    for book in BOOKS:
        books_dict[book["number"]] = {
            "number": book["number"],
            "name": book["name"],
            "abbreviation": book["abbr"],
            "chapters": {}
        }
    
    lines = text.split('\n')
    total_lines = len(lines)
    processed = 0
    
    log_print(f"Processing {total_lines} lines...")
    
    for line in lines:
        line = line.strip()
        if not line:
            continue
        
        # Skip header lines
        if line.startswith('KJV') or line.startswith('King James') or line.startswith('Text courtesy'):
            continue
        
        # Parse verse: "Genesis 1:1	In the beginning..."
        # Format: BookName Chapter:Verse\tText
        if '\t' not in line:
            continue
        
        parts = line.split('\t', 1)
        if len(parts) != 2:
            continue
        
        reference = parts[0].strip()
        verse_text = parts[1].strip()
        
        # Remove [ ] brackets but keep the content inside
        verse_text = verse_text.replace('[', '').replace(']', '')
        # Clean up extra spaces that might result from removal
        verse_text = re.sub(r'\s+', ' ', verse_text).strip()
        
        # Parse reference: "Genesis 1:1" or "1 Samuel 1:1"
        match = re.match(r'^(.+?)\s+(\d+):(\d+)$', reference)
        if not match:
            continue
        
        book_name = match.group(1).strip()
        chapter_num = int(match.group(2))
        verse_num = int(match.group(3))
        
        # Find book number
        book_num = BOOK_NAME_MAP.get(book_name.lower())
        if not book_num:
            # Try to find by exact match with different spacing
            book_name_no_space = book_name.replace(" ", "").lower()
            book_num = BOOK_NAME_MAP.get(book_name_no_space)
        
        if not book_num:
            log_print(f"WARNING: Unknown book name: {book_name} (reference: {reference})")
            continue
        
        # Add verse to structure
        if book_num not in books_dict:
            continue
        
        book = books_dict[book_num]
        if chapter_num not in book["chapters"]:
            book["chapters"][chapter_num] = []
        
        book["chapters"][chapter_num].append({
            "number": verse_num,
            "text": verse_text
        })
        
        processed += 1
        if processed % 1000 == 0:
            log_print(f"  Processed {processed} verses...")
    
    log_print(f"✅ Processed {processed} verses total")
    
    # Convert to final structure
    for book_num in sorted(books_dict.keys()):
        book = books_dict[book_num]
        chapters_list = []
        
        for chapter_num in sorted(book["chapters"].keys()):
            verses = book["chapters"][chapter_num]
            # Sort verses by number
            verses.sort(key=lambda x: x["number"])
            chapters_list.append({
                "number": chapter_num,
                "verses": verses
            })
        
        if chapters_list:
            output_data["books"].append({
                "number": book["number"],
                "name": book["name"],
                "abbreviation": book["abbreviation"],
                "chapters": chapters_list
            })
    
    return output_data


def save_json(output_file: str, data: Dict):
    """Save data to JSON file"""
    log_print(f"Saving to {output_file}...")
    try:
        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        log_print(f"✅ Successfully saved to {output_file}")
    except Exception as e:
        log_print(f"ERROR: Failed to save JSON: {e}", file=sys.stderr)
        sys.exit(1)


def main():
    import argparse
    
    parser = argparse.ArgumentParser(
        description='Fetch KJV Bible from OpenBible.com text file',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Fetch complete KJV
  %(prog)s -o bible_kjv.json
  
  # Fetch with log file
  %(prog)s -o bible_kjv.json -l fetch_kjv.log
        """
    )
    parser.add_argument('-o', '--output', default='bible_kjv.json', help='Output JSON file')
    parser.add_argument('-l', '--log', help='Log file path (optional)')
    
    args = parser.parse_args()
    
    # Initialize log file if specified
    if args.log:
        init_log_file(args.log)
    
    try:
        # Fetch text
        text = fetch_kjv_text()
        
        # Parse text
        data = parse_kjv_text(text)
        
        # Save JSON
        save_json(args.output, data)
        
        # Print summary
        total_verses = 0
        for book in data["books"]:
            for chapter in book["chapters"]:
                total_verses += len(chapter["verses"])
        
        log_print(f"\n{'='*60}")
        log_print(f"Complete!")
        log_print(f"  Version: {data['version']['name']}")
        log_print(f"  Books: {len(data['books'])}")
        log_print(f"  Verses: {total_verses}")
        log_print(f"  Output: {args.output}")
        log_print(f"{'='*60}")
        
    except KeyboardInterrupt:
        log_print("\n\nInterrupted by user.")
        close_log_file()
        sys.exit(1)
    except Exception as e:
        log_print(f"\nERROR: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        close_log_file()
        sys.exit(1)
    finally:
        close_log_file()


if __name__ == '__main__':
    main()

