#!/usr/bin/env python3
"""
Bible Scraper for BibleStudyTools.com
Fetches NIV, NKJV and other versions from https://www.biblestudytools.com/
"""

import requests
from bs4 import BeautifulSoup
import json
import time
import sys
import re
from typing import List, Dict, Optional
from datetime import datetime

# English book names with URL slugs and chapter counts
BOOKS = [
    {"number": 1, "name": "Genesis", "slug": "genesis", "abbr": "Gen", "chapters": 50},
    {"number": 2, "name": "Exodus", "slug": "exodus", "abbr": "Exod", "chapters": 40},
    {"number": 3, "name": "Leviticus", "slug": "leviticus", "abbr": "Lev", "chapters": 27},
    {"number": 4, "name": "Numbers", "slug": "numbers", "abbr": "Num", "chapters": 36},
    {"number": 5, "name": "Deuteronomy", "slug": "deuteronomy", "abbr": "Deut", "chapters": 34},
    {"number": 6, "name": "Joshua", "slug": "joshua", "abbr": "Josh", "chapters": 24},
    {"number": 7, "name": "Judges", "slug": "judges", "abbr": "Judg", "chapters": 21},
    {"number": 8, "name": "Ruth", "slug": "ruth", "abbr": "Ruth", "chapters": 4},
    {"number": 9, "name": "1 Samuel", "slug": "1-samuel", "abbr": "1Sam", "chapters": 31},
    {"number": 10, "name": "2 Samuel", "slug": "2-samuel", "abbr": "2Sam", "chapters": 24},
    {"number": 11, "name": "1 Kings", "slug": "1-kings", "abbr": "1Kgs", "chapters": 22},
    {"number": 12, "name": "2 Kings", "slug": "2-kings", "abbr": "2Kgs", "chapters": 25},
    {"number": 13, "name": "1 Chronicles", "slug": "1-chronicles", "abbr": "1Chr", "chapters": 29},
    {"number": 14, "name": "2 Chronicles", "slug": "2-chronicles", "abbr": "2Chr", "chapters": 36},
    {"number": 15, "name": "Ezra", "slug": "ezra", "abbr": "Ezra", "chapters": 10},
    {"number": 16, "name": "Nehemiah", "slug": "nehemiah", "abbr": "Neh", "chapters": 13},
    {"number": 17, "name": "Esther", "slug": "esther", "abbr": "Esth", "chapters": 10},
    {"number": 18, "name": "Job", "slug": "job", "abbr": "Job", "chapters": 42},
    {"number": 19, "name": "Psalms", "slug": "psalms", "abbr": "Ps", "chapters": 150},
    {"number": 20, "name": "Proverbs", "slug": "proverbs", "abbr": "Prov", "chapters": 31},
    {"number": 21, "name": "Ecclesiastes", "slug": "ecclesiastes", "abbr": "Eccl", "chapters": 12},
    {"number": 22, "name": "Song of Solomon", "slug": "song-of-solomon", "abbr": "Song", "chapters": 8},
    {"number": 23, "name": "Isaiah", "slug": "isaiah", "abbr": "Isa", "chapters": 66},
    {"number": 24, "name": "Jeremiah", "slug": "jeremiah", "abbr": "Jer", "chapters": 52},
    {"number": 25, "name": "Lamentations", "slug": "lamentations", "abbr": "Lam", "chapters": 5},
    {"number": 26, "name": "Ezekiel", "slug": "ezekiel", "abbr": "Ezek", "chapters": 48},
    {"number": 27, "name": "Daniel", "slug": "daniel", "abbr": "Dan", "chapters": 12},
    {"number": 28, "name": "Hosea", "slug": "hosea", "abbr": "Hos", "chapters": 14},
    {"number": 29, "name": "Joel", "slug": "joel", "abbr": "Joel", "chapters": 3},
    {"number": 30, "name": "Amos", "slug": "amos", "abbr": "Amos", "chapters": 9},
    {"number": 31, "name": "Obadiah", "slug": "obadiah", "abbr": "Obad", "chapters": 1},
    {"number": 32, "name": "Jonah", "slug": "jonah", "abbr": "Jonah", "chapters": 4},
    {"number": 33, "name": "Micah", "slug": "micah", "abbr": "Mic", "chapters": 7},
    {"number": 34, "name": "Nahum", "slug": "nahum", "abbr": "Nah", "chapters": 3},
    {"number": 35, "name": "Habakkuk", "slug": "habakkuk", "abbr": "Hab", "chapters": 3},
    {"number": 36, "name": "Zephaniah", "slug": "zephaniah", "abbr": "Zeph", "chapters": 3},
    {"number": 37, "name": "Haggai", "slug": "haggai", "abbr": "Hag", "chapters": 2},
    {"number": 38, "name": "Zechariah", "slug": "zechariah", "abbr": "Zech", "chapters": 14},
    {"number": 39, "name": "Malachi", "slug": "malachi", "abbr": "Mal", "chapters": 4},
    {"number": 40, "name": "Matthew", "slug": "matthew", "abbr": "Matt", "chapters": 28},
    {"number": 41, "name": "Mark", "slug": "mark", "abbr": "Mark", "chapters": 16},
    {"number": 42, "name": "Luke", "slug": "luke", "abbr": "Luke", "chapters": 24},
    {"number": 43, "name": "John", "slug": "john", "abbr": "John", "chapters": 21},
    {"number": 44, "name": "Acts", "slug": "acts", "abbr": "Acts", "chapters": 28},
    {"number": 45, "name": "Romans", "slug": "romans", "abbr": "Rom", "chapters": 16},
    {"number": 46, "name": "1 Corinthians", "slug": "1-corinthians", "abbr": "1Cor", "chapters": 16},
    {"number": 47, "name": "2 Corinthians", "slug": "2-corinthians", "abbr": "2Cor", "chapters": 13},
    {"number": 48, "name": "Galatians", "slug": "galatians", "abbr": "Gal", "chapters": 6},
    {"number": 49, "name": "Ephesians", "slug": "ephesians", "abbr": "Eph", "chapters": 6},
    {"number": 50, "name": "Philippians", "slug": "philippians", "abbr": "Phil", "chapters": 4},
    {"number": 51, "name": "Colossians", "slug": "colossians", "abbr": "Col", "chapters": 4},
    {"number": 52, "name": "1 Thessalonians", "slug": "1-thessalonians", "abbr": "1Thess", "chapters": 5},
    {"number": 53, "name": "2 Thessalonians", "slug": "2-thessalonians", "abbr": "2Thess", "chapters": 3},
    {"number": 54, "name": "1 Timothy", "slug": "1-timothy", "abbr": "1Tim", "chapters": 6},
    {"number": 55, "name": "2 Timothy", "slug": "2-timothy", "abbr": "2Tim", "chapters": 4},
    {"number": 56, "name": "Titus", "slug": "titus", "abbr": "Titus", "chapters": 3},
    {"number": 57, "name": "Philemon", "slug": "philemon", "abbr": "Phlm", "chapters": 1},
    {"number": 58, "name": "Hebrews", "slug": "hebrews", "abbr": "Heb", "chapters": 13},
    {"number": 59, "name": "James", "slug": "james", "abbr": "Jas", "chapters": 5},
    {"number": 60, "name": "1 Peter", "slug": "1-peter", "abbr": "1Pet", "chapters": 5},
    {"number": 61, "name": "2 Peter", "slug": "2-peter", "abbr": "2Pet", "chapters": 3},
    {"number": 62, "name": "1 John", "slug": "1-john", "abbr": "1John", "chapters": 5},
    {"number": 63, "name": "2 John", "slug": "2-john", "abbr": "2John", "chapters": 1},
    {"number": 64, "name": "3 John", "slug": "3-john", "abbr": "3John", "chapters": 1},
    {"number": 65, "name": "Jude", "slug": "jude", "abbr": "Jude", "chapters": 1},
    {"number": 66, "name": "Revelation", "slug": "revelation", "abbr": "Rev", "chapters": 22},
]

VERSION_NAMES = {
    "niv": "New International Version",
    "nkjv": "New King James Version",
    "kjv": "King James Version",
    "esv": "English Standard Version",
    "nasb": "New American Standard Bible",
    "nlt": "New Living Translation",
}

BASE_URL = "https://www.biblestudytools.com"

HEADERS = {
    "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
    "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    "Accept-Language": "en-US,en;q=0.5",
}

# Global log file handle
_log_file: Optional[object] = None


def log_print(*args, **kwargs):
    """Print to both console and log file"""
    # Print to console
    print(*args, **kwargs)
    
    # Write to log file if available
    if _log_file:
        # Remove end parameter if present (for flush behavior)
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


def fetch_chapter(book_slug: str, chapter: int, translation: str = "niv", max_retries: int = 3) -> List[Dict]:
    """Fetch a chapter from BibleStudyTools.com with retry mechanism"""
    url = f"{BASE_URL}/{translation}/{book_slug}/{chapter}.html"
    if translation == "niv":
        url = f"{BASE_URL}/{book_slug}/{chapter}.html"
    
    for attempt in range(1, max_retries + 1):
        try:
            if attempt > 1:
                log_print(f"  Retry {attempt}/{max_retries}...", end=" ", flush=True)
                time.sleep(2)  # Wait before retry
            else:
                log_print(f"  Fetching chapter {chapter}...", end=" ", flush=True)
            
            response = requests.get(url, headers=HEADERS, timeout=30)
            response.raise_for_status()
            
            soup = BeautifulSoup(response.text, 'html.parser')
            verses = []
            
            # Find all divs with data-verse-id attribute
            verse_divs = soup.find_all('div', {'data-verse-id': True})
            
            for div in verse_divs:
                verse_id = div.get('data-verse-id')
                if not verse_id:
                    continue
                
                try:
                    verse_num = int(verse_id)
                except ValueError:
                    continue
                
                # Remove unwanted elements (h3 headings, links, etc.)
                # Note: Keep span elements as they contain verse text (especially red-letter class)
                for unwanted in div.find_all(['h3', 'a']):
                    unwanted.decompose()
                
                # Get the verse text (spans are kept as they contain the actual verse content)
                verse_text = div.get_text(strip=True)
                
                # Clean up the text
                verse_text = clean_verse_text(verse_text)
                
                if verse_text and len(verse_text) > 3:
                    verses.append({
                        "number": verse_num,
                        "text": verse_text
                    })
            
            # Sort by verse number
            verses = sorted(verses, key=lambda x: x["number"])
            
            if verses:
                log_print(f"OK ({len(verses)} verses)")
                return verses
            else:
                # No verses found, will retry if attempts remain
                if attempt < max_retries:
                    continue
                else:
                    log_print(f"ERROR: No verses found after {max_retries} attempts")
                    return []
        
        except requests.RequestException as e:
            if attempt < max_retries:
                log_print(f"ERROR: {e}, retrying...")
                continue
            else:
                log_print(f"ERROR: {e} (after {max_retries} attempts)")
                return []
        except Exception as e:
            if attempt < max_retries:
                log_print(f"ERROR: {e}, retrying...")
                continue
            else:
                log_print(f"ERROR: {e} (after {max_retries} attempts)")
                import traceback
                traceback.print_exc()
                return []
    
    return []


def clean_verse_text(text: str) -> str:
    """Clean up verse text"""
    # Remove extra whitespace
    text = re.sub(r'\s+', ' ', text)
    # Remove footnote markers like [a], [b], etc.
    text = re.sub(r'\[[a-z]\]', '', text)
    text = re.sub(r'\[\d+\]', '', text)
    return text.strip()


def fetch_book(book: Dict, translation: str = "niv") -> Dict:
    """Fetch all chapters of a book"""
    log_print(f"\nFetching: {book['name']} ({book['chapters']} chapters)")
    
    book_data = {
        "number": book["number"],
        "name": book["name"],
        "abbreviation": book["abbr"],
        "chapters": []
    }
    
    for chapter_num in range(1, book["chapters"] + 1):
        verses = fetch_chapter(book["slug"], chapter_num, translation)
        
        if verses:
            book_data["chapters"].append({
                "number": chapter_num,
                "verses": verses
            })
        else:
            log_print(f"  WARNING: No verses found for chapter {chapter_num}")
        
        time.sleep(1.0)
    
    return book_data


def fetch_bible(output_file: str, translation: str = "niv", start_from: int = 1, end_at: int = 66):
    """Fetch complete Bible from BibleStudyTools.com"""
    version_name = VERSION_NAMES.get(translation, translation.upper())
    
    output_data = {
        "version": {
            "code": translation.upper(),
            "name": version_name
        },
        "books": []
    }
    
    total_verses = 0
    books_to_fetch = [b for b in BOOKS if start_from <= b["number"] <= end_at]
    
    log_print(f"{'='*60}")
    log_print(f"Fetching {version_name} from BibleStudyTools.com")
    log_print(f"Books: {start_from} to {end_at} ({len(books_to_fetch)} books)")
    log_print(f"{'='*60}")
    
    for i, book in enumerate(books_to_fetch, 1):
        log_print(f"\n[{i}/{len(books_to_fetch)}]", end=" ")
        book_data = fetch_book(book, translation)
        
        if book_data["chapters"]:
            output_data["books"].append(book_data)
            for chapter in book_data["chapters"]:
                total_verses += len(chapter["verses"])
        else:
            log_print(f"  WARNING: No content for {book['name']}")
        
        # Save progress after each book
        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump(output_data, f, ensure_ascii=False, indent=2)
    
    log_print(f"\n{'='*60}")
    log_print(f"Complete!")
    log_print(f"  Version: {version_name}")
    log_print(f"  Books: {len(output_data['books'])}")
    log_print(f"  Verses: {total_verses}")
    log_print(f"  Output: {output_file}")
    log_print(f"{'='*60}")


def retry_chapter(book_number: int, chapter_number: int, json_file: str, translation: str = "niv"):
    """Retry fetching a single chapter and merge into existing JSON file"""
    book = next((b for b in BOOKS if b["number"] == book_number), None)
    if not book:
        log_print(f"ERROR: Book number {book_number} not found", file=sys.stderr)
        sys.exit(1)
    
    if chapter_number < 1 or chapter_number > book["chapters"]:
        log_print(f"ERROR: Chapter {chapter_number} not valid for {book['name']} (max: {book['chapters']})", file=sys.stderr)
        sys.exit(1)
    
    # Load existing JSON file
    try:
        with open(json_file, 'r', encoding='utf-8') as f:
            output_data = json.load(f)
    except FileNotFoundError:
        log_print(f"ERROR: JSON file not found: {json_file}", file=sys.stderr)
        sys.exit(1)
    except json.JSONDecodeError as e:
        log_print(f"ERROR: Invalid JSON file: {e}", file=sys.stderr)
        sys.exit(1)
    
    version_name = VERSION_NAMES.get(translation, translation.upper())
    
    log_print(f"{'='*60}")
    log_print(f"Retrying: {book['name']} Chapter {chapter_number}")
    log_print(f"Translation: {version_name} ({translation.upper()})")
    log_print(f"Target file: {json_file}")
    log_print(f"{'='*60}\n")
    
    # Fetch the chapter
    verses = fetch_chapter(book["slug"], chapter_number, translation)
    
    if not verses:
        log_print(f"\nERROR: No verses found for {book['name']} {chapter_number}")
        log_print("Original file unchanged.")
        sys.exit(1)
    
    # Find the book in the existing data
    book_found = False
    for b in output_data["books"]:
        if b["number"] == book_number:
            book_found = True
            # Find or create the chapter
            chapter_found = False
            for ch in b["chapters"]:
                if ch["number"] == chapter_number:
                    # Replace existing chapter
                    ch["verses"] = verses
                    chapter_found = True
                    log_print(f"  Updated existing chapter {chapter_number}")
                    break
            
            if not chapter_found:
                # Add new chapter (maintain sorted order)
                b["chapters"].append({
                    "number": chapter_number,
                    "verses": verses
                })
                # Sort chapters by number
                b["chapters"].sort(key=lambda x: x["number"])
                log_print(f"  Added new chapter {chapter_number}")
            break
    
    if not book_found:
        # Book doesn't exist, add it
        output_data["books"].append({
            "number": book["number"],
            "name": book["name"],
            "abbreviation": book["abbr"],
            "chapters": [{
                "number": chapter_number,
                "verses": verses
            }]
        })
        # Sort books by number
        output_data["books"].sort(key=lambda x: x["number"])
        log_print(f"  Added new book: {book['name']}")
    
    # Save the updated JSON file
    try:
        with open(json_file, 'w', encoding='utf-8') as f:
            json.dump(output_data, f, ensure_ascii=False, indent=2)
        log_print(f"\n✅ Successfully updated {json_file}")
    except Exception as e:
        log_print(f"\nERROR: Failed to save file: {e}", file=sys.stderr)
        sys.exit(1)
    
    log_print(f"\n{'='*60}")
    log_print(f"Retry Complete!")
    log_print(f"  Book: {book['name']}")
    log_print(f"  Chapter: {chapter_number}")
    log_print(f"  Verses: {len(verses)}")
    log_print(f"  Verse numbers: {[v['number'] for v in verses]}")
    log_print(f"  Updated file: {json_file}")
    log_print(f"{'='*60}")
    
    # Show first and last verse
    if verses:
        log_print(f"\nFirst verse ({verses[0]['number']}):")
        log_print(f"  {verses[0]['text']}")
        log_print(f"\nLast verse ({verses[-1]['number']}):")
        log_print(f"  {verses[-1]['text']}")


def test_chapter(book_number: int, chapter_number: int, translation: str = "niv", output_file: str = "test_chapter.json"):
    """Test fetching a single chapter"""
    book = next((b for b in BOOKS if b["number"] == book_number), None)
    if not book:
        log_print(f"ERROR: Book number {book_number} not found", file=sys.stderr)
        sys.exit(1)
    
    if chapter_number < 1 or chapter_number > book["chapters"]:
        log_print(f"ERROR: Chapter {chapter_number} not valid for {book['name']} (max: {book['chapters']})", file=sys.stderr)
        sys.exit(1)
    
    version_name = VERSION_NAMES.get(translation, translation.upper())
    
    log_print(f"{'='*60}")
    log_print(f"Testing: {book['name']} Chapter {chapter_number}")
    log_print(f"Translation: {version_name} ({translation.upper()})")
    log_print(f"{'='*60}\n")
    
    verses = fetch_chapter(book["slug"], chapter_number, translation)
    
    if not verses:
        log_print(f"\nERROR: No verses found for {book['name']} {chapter_number}")
        sys.exit(1)
    
    output_data = {
        "version": {
            "code": translation.upper(),
            "name": version_name
        },
        "books": [
            {
                "number": book["number"],
                "name": book["name"],
                "abbreviation": book["abbr"],
                "chapters": [
                    {
                        "number": chapter_number,
                        "verses": verses
                    }
                ]
            }
        ]
    }
    
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(output_data, f, ensure_ascii=False, indent=2)
    
    log_print(f"\n{'='*60}")
    log_print(f"Test Complete!")
    log_print(f"  Book: {book['name']}")
    log_print(f"  Chapter: {chapter_number}")
    log_print(f"  Verses: {len(verses)}")
    log_print(f"  Verse numbers: {[v['number'] for v in verses]}")
    log_print(f"  Output: {output_file}")
    log_print(f"{'='*60}")
    
    # Show first and last verse
    if verses:
        log_print(f"\nFirst verse ({verses[0]['number']}):")
        log_print(f"  {verses[0]['text']}")
        log_print(f"\nLast verse ({verses[-1]['number']}):")
        log_print(f"  {verses[-1]['text']}")


def main():
    import argparse
    
    parser = argparse.ArgumentParser(
        description='Fetch Bible from BibleStudyTools.com',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Test a single chapter (Genesis 1)
  %(prog)s --test-chapter 1 1 -t niv
  
  # Fetch complete NIV
  %(prog)s -o bible_niv.json -t niv
  
  # Fetch only Genesis (book 1)
  %(prog)s -o bible_niv_genesis.json -t niv -s 1 -e 1
  
  # Retry a failed chapter and merge into existing file
  %(prog)s --retry-chapter 1 5 -o bible_niv.json -t niv

Available translations:
  - niv:  New International Version
  - nkjv: New King James Version
  - kjv:  King James Version
  - esv:  English Standard Version
  - nasb: New American Standard Bible
  - nlt:  New Living Translation
        """
    )
    parser.add_argument('-o', '--output', default='bible_niv.json', help='Output file')
    parser.add_argument('-t', '--translation', default='niv', help='Translation code (niv, nkjv, kjv, etc.)')
    parser.add_argument('-s', '--start', type=int, default=1, help='Start book (1-66)')
    parser.add_argument('-e', '--end', type=int, default=66, help='End book (1-66)')
    parser.add_argument('--test-chapter', nargs=2, metavar=('BOOK', 'CHAPTER'), 
                       type=int, help='Test mode: fetch only specified book and chapter (e.g., --test-chapter 1 1 for Genesis 1)')
    parser.add_argument('--retry-chapter', nargs=2, metavar=('BOOK', 'CHAPTER'),
                       type=int, help='Retry mode: retry a specific chapter and merge into existing JSON file (requires -o/--output)')
    parser.add_argument('-l', '--log', help='Log file path (optional, if not specified logs will only go to console)')
    
    args = parser.parse_args()
    
    # Initialize log file if specified
    if args.log:
        init_log_file(args.log)
    
    # Retry mode
    if args.retry_chapter:
        if not args.output:
            log_print("ERROR: --retry-chapter requires -o/--output to specify the JSON file to update", file=sys.stderr)
            close_log_file()
            sys.exit(1)
        
        book_num, chapter_num = args.retry_chapter
        if book_num < 1 or book_num > 66:
            log_print("ERROR: Book number must be between 1-66", file=sys.stderr)
            close_log_file()
            sys.exit(1)
        
        try:
            retry_chapter(book_num, chapter_num, args.output, args.translation.lower())
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
        return
    
    # Test mode
    if args.test_chapter:
        book_num, chapter_num = args.test_chapter
        if book_num < 1 or book_num > 66:
            log_print("ERROR: Book number must be between 1-66", file=sys.stderr)
            close_log_file()
            sys.exit(1)
        
        try:
            test_chapter(book_num, chapter_num, args.translation.lower(), args.output)
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
        return
    
    # Normal mode
    if args.start < 1 or args.start > 66 or args.end < 1 or args.end > 66:
        log_print("ERROR: Book numbers must be between 1-66", file=sys.stderr)
        close_log_file()
        sys.exit(1)
    
    if args.start > args.end:
        log_print("ERROR: Start book cannot be greater than end book", file=sys.stderr)
        close_log_file()
        sys.exit(1)
    
    try:
        fetch_bible(args.output, args.translation.lower(), args.start, args.end)
    except KeyboardInterrupt:
        log_print("\n\nInterrupted by user. Progress has been saved.")
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

