#!/usr/bin/env python3
import requests
from bs4 import BeautifulSoup
import json
import time
import sys
from typing import List, Dict
import opencc

# 聖經書卷列表（66卷）
BOOKS = [
    # 舊約（39卷）
    {"number": 1, "name": "創世記", "short": "創", "chapters": 50},
    {"number": 2, "name": "出埃及記", "short": "出", "chapters": 40},
    {"number": 3, "name": "利未記", "short": "利", "chapters": 27},
    {"number": 4, "name": "民數記", "short": "民", "chapters": 36},
    {"number": 5, "name": "申命記", "short": "申", "chapters": 34},
    {"number": 6, "name": "約書亞記", "short": "書", "chapters": 24},
    {"number": 7, "name": "士師記", "short": "士", "chapters": 21},
    {"number": 8, "name": "路得記", "short": "得", "chapters": 4},
    {"number": 9, "name": "撒母耳記上", "short": "撒上", "chapters": 31},
    {"number": 10, "name": "撒母耳記下", "short": "撒下", "chapters": 24},
    {"number": 11, "name": "列王紀上", "short": "王上", "chapters": 22},
    {"number": 12, "name": "列王紀下", "short": "王下", "chapters": 25},
    {"number": 13, "name": "歷代志上", "short": "代上", "chapters": 29},
    {"number": 14, "name": "歷代志下", "short": "代下", "chapters": 36},
    {"number": 15, "name": "以斯拉記", "short": "拉", "chapters": 10},
    {"number": 16, "name": "尼希米記", "short": "尼", "chapters": 13},
    {"number": 17, "name": "以斯帖記", "short": "斯", "chapters": 10},
    {"number": 18, "name": "約伯記", "short": "伯", "chapters": 42},
    {"number": 19, "name": "詩篇", "short": "詩", "chapters": 150},
    {"number": 20, "name": "箴言", "short": "箴", "chapters": 31},
    {"number": 21, "name": "傳道書", "short": "傳", "chapters": 12},
    {"number": 22, "name": "雅歌", "short": "歌", "chapters": 8},
    {"number": 23, "name": "以賽亞書", "short": "賽", "chapters": 66},
    {"number": 24, "name": "耶利米書", "short": "耶", "chapters": 52},
    {"number": 25, "name": "耶利米哀歌", "short": "哀", "chapters": 5},
    {"number": 26, "name": "以西結書", "short": "結", "chapters": 48},
    {"number": 27, "name": "但以理書", "short": "但", "chapters": 12},
    {"number": 28, "name": "何西阿書", "short": "何", "chapters": 14},
    {"number": 29, "name": "約珥書", "short": "珥", "chapters": 3},
    {"number": 30, "name": "阿摩司書", "short": "摩", "chapters": 9},
    {"number": 31, "name": "俄巴底亞書", "short": "俄", "chapters": 1},
    {"number": 32, "name": "約拿書", "short": "拿", "chapters": 4},
    {"number": 33, "name": "彌迦書", "short": "彌", "chapters": 7},
    {"number": 34, "name": "那鴻書", "short": "鴻", "chapters": 3},
    {"number": 35, "name": "哈巴谷書", "short": "哈", "chapters": 3},
    {"number": 36, "name": "西番雅書", "short": "番", "chapters": 3},
    {"number": 37, "name": "哈該書", "short": "該", "chapters": 2},
    {"number": 38, "name": "撒迦利亞書", "short": "亞", "chapters": 14},
    {"number": 39, "name": "瑪拉基書", "short": "瑪", "chapters": 4},
    # 新約（27卷）
    {"number": 40, "name": "馬太福音", "short": "太", "chapters": 28},
    {"number": 41, "name": "馬可福音", "short": "可", "chapters": 16},
    {"number": 42, "name": "路加福音", "short": "路", "chapters": 24},
    {"number": 43, "name": "約翰福音", "short": "約", "chapters": 21},
    {"number": 44, "name": "使徒行傳", "short": "徒", "chapters": 28},
    {"number": 45, "name": "羅馬書", "short": "羅", "chapters": 16},
    {"number": 46, "name": "哥林多前書", "short": "林前", "chapters": 16},
    {"number": 47, "name": "哥林多後書", "short": "林後", "chapters": 13},
    {"number": 48, "name": "加拉太書", "short": "加", "chapters": 6},
    {"number": 49, "name": "以弗所書", "short": "弗", "chapters": 6},
    {"number": 50, "name": "腓立比書", "short": "腓", "chapters": 4},
    {"number": 51, "name": "歌羅西書", "short": "西", "chapters": 4},
    {"number": 52, "name": "帖撒羅尼迦前書", "short": "帖前", "chapters": 5},
    {"number": 53, "name": "帖撒羅尼迦後書", "short": "帖後", "chapters": 3},
    {"number": 54, "name": "提摩太前書", "short": "提前", "chapters": 6},
    {"number": 55, "name": "提摩太後書", "short": "提後", "chapters": 4},
    {"number": 56, "name": "提多書", "short": "多", "chapters": 3},
    {"number": 57, "name": "腓利門書", "short": "門", "chapters": 1},
    {"number": 58, "name": "希伯來書", "short": "來", "chapters": 13},
    {"number": 59, "name": "雅各書", "short": "雅", "chapters": 5},
    {"number": 60, "name": "彼得前書", "short": "彼前", "chapters": 5},
    {"number": 61, "name": "彼得後書", "short": "彼後", "chapters": 3},
    {"number": 62, "name": "約翰一書", "short": "約一", "chapters": 5},
    {"number": 63, "name": "約翰二書", "short": "約二", "chapters": 1},
    {"number": 64, "name": "約翰三書", "short": "約三", "chapters": 1},
    {"number": 65, "name": "猶大書", "short": "猶", "chapters": 1},
    {"number": 66, "name": "啟示錄", "short": "啟", "chapters": 22},
]

BASE_URL = "https://cb.fhl.net/read1.php"


def fetch_chapter(book_short: str, chapter: int) -> List[Dict]:
    """
    從信望愛網站獲取一章經文
    
    Args:
        book_short: 書卷簡稱（如：創、出）
        chapter: 章節號碼
    
    Returns:
        經文列表 [{"number": 1, "text": "經文內容"}, ...]
    """
    params = {
        "VERSION22": "scunp89",  # 新標點和合本神版
        "TABFLAG": "1",
        "chineses": book_short,
        "chap": chapter,
        "submit1": "閱讀"
    }
    
    try:
        print(f"  正在獲取 {book_short} 第 {chapter} 章...", end=" ", flush=True)
        response = requests.get(BASE_URL, params=params, timeout=30)
        response.raise_for_status()
        response.encoding = 'utf-8'
        
        # 解析 HTML
        soup = BeautifulSoup(response.text, 'html.parser')
        
        verses = []
        
        # 找到經文表格
        table = soup.find('table')
        if not table:
            print("❌ 未找到經文表格")
            return []
        
        rows = table.find_all('tr')
        
        for row in rows:
            cols = row.find_all('td')
            if len(cols) >= 2:
                # 第一列是章:節，第二列是經文
                verse_ref = cols[0].get_text(strip=True)
                
                # 第二列可能包含多個元素，需要小心提取
                verse_cell = cols[1]
                
                # 移除粗體標籤（小標題）
                for bold in verse_cell.find_all('b'):
                    bold.decompose()
                
                # 移除 strong 標籤（小標題）
                for strong in verse_cell.find_all('strong'):
                    strong.decompose()
                
                # 移除參考連結（包含 <a> 標籤的內容）
                for link in verse_cell.find_all('a'):
                    link.decompose()
                
                # 移除 <br/> 標籤
                for br in verse_cell.find_all('br'):
                    br.decompose()
                
                verse_text = verse_cell.get_text(strip=True)
                
                # 解析經節號碼（格式：1:1）
                if ':' in verse_ref:
                    parts = verse_ref.split(':')
                    if len(parts) == 2:
                        try:
                            verse_num = int(parts[1])
                            
                            if verse_text:
                                # 清理經文內容
                                cleaned_text = verse_text
                                
                                # 移除「神」、「上帝」前面的空格（和合本排版特色）
                                cleaned_text = cleaned_text.replace(' 神', '神')
                                cleaned_text = cleaned_text.replace(' 上帝', '上帝')
                                
                                # 移除全形空格前的「神」
                                cleaned_text = cleaned_text.replace('　神', '神')
                                cleaned_text = cleaned_text.replace('　上帝', '上帝')
                                
                                verses.append({
                                    "number": verse_num,
                                    "text": cleaned_text
                                })
                        except ValueError:
                            continue
        
        print(f"✓ ({len(verses)} 節)")
        return verses
        
    except requests.RequestException as e:
        print(f"❌ 網路錯誤: {e}")
        return []
    except Exception as e:
        print(f"❌ 解析錯誤: {e}")
        return []


def fetch_book(book: Dict) -> Dict:
    """
    獲取一卷書的所有章節
    
    Args:
        book: 書卷資訊字典
    
    Returns:
        包含所有章節的書卷資料
    """
    print(f"\n📖 正在獲取：{book['name']} ({book['chapters']} 章)")
    
    book_data = {
        "number": book["number"],
        "name": book["name"],
        "abbreviation": book["short"],
        "chapters": []
    }
    
    for chapter_num in range(1, book["chapters"] + 1):
        verses = fetch_chapter(book["short"], chapter_num)
        
        if verses:
            book_data["chapters"].append({
                "number": chapter_num,
                "verses": verses
            })
        
        # 禮貌性延遲，避免對伺服器造成負擔
        time.sleep(0.5)
    
    return book_data


def convert_to_simplified(data: Dict) -> Dict:
    """
    將繁體版本轉換為簡體版本
    
    Args:
        data: 繁體版本的聖經資料
    
    Returns:
        簡體版本的聖經資料
    """
    print("\n🔄 正在轉換為簡體版本...")
    
    # 初始化 OpenCC 轉換器
    converter = opencc.OpenCC('t2s')  # 繁體轉簡體
    
    simplified_data = {
        "version": {
            "code": data["version"]["code"] + "-SC",  # 加上 -SC 後綴
            "name": converter.convert(data["version"]["name"]) + " (简体)"
        },
        "books": [],
        "total_books": data["total_books"],
        "total_verses": data["total_verses"]
    }
    
    for book in data["books"]:
        simplified_book = {
            "number": book["number"],
            "name": converter.convert(book["name"]),
            "abbreviation": converter.convert(book["abbreviation"]),
            "chapters": []
        }
        
        for chapter in book["chapters"]:
            simplified_chapter = {
                "number": chapter["number"],
                "verses": []
            }
            
            for verse in chapter["verses"]:
                simplified_chapter["verses"].append({
                    "number": verse["number"],
                    "text": converter.convert(verse["text"])
                })
            
            simplified_book["chapters"].append(simplified_chapter)
        
        simplified_data["books"].append(simplified_book)
    
    print(f"✅ 簡體轉換完成")
    return simplified_data


def fetch_all_books(output_file: str = "bible_cunp89.json", start_from: int = 1, end_at: int = 66):
    """
    獲取所有書卷並儲存為 JSON
    
    Args:
        output_file: 輸出檔案名稱
        start_from: 從第幾卷開始（1-66）
        end_at: 到第幾卷結束（1-66）
    """
    output_data = {
        "version": {
            "code": "CUNP89",
            "name": "新標點和合本神版"
        },
        "books": [],
        "total_books": 0,
        "total_verses": 0
    }
    
    total_verses = 0
    
    # 過濾要下載的書卷
    books_to_fetch = [b for b in BOOKS if start_from <= b["number"] <= end_at]
    
    print(f"=== 開始抓取新標點和合本神版 ===")
    print(f"書卷範圍: 第 {start_from} 卷到第 {end_at} 卷（共 {len(books_to_fetch)} 卷）")
    print(f"來源: https://cb.fhl.net/")
    print(f"版權: 台灣聖經公會\n")
    
    for i, book in enumerate(books_to_fetch, 1):
        print(f"[{i}/{len(books_to_fetch)}]", end=" ")
        book_data = fetch_book(book)
        
        if book_data["chapters"]:
            output_data["books"].append(book_data)
            
            # 統計經文數量
            for chapter in book_data["chapters"]:
                total_verses += len(chapter["verses"])
        else:
            print(f"⚠️  警告：{book['name']} 未獲取到任何內容")
    
    output_data["total_books"] = len(output_data["books"])
    output_data["total_verses"] = total_verses
    
    # 儲存繁體版本
    with open(output_file, 'w', encoding='utf-8') as f:
        json.dump(output_data, f, ensure_ascii=False, indent=2)
    
    print(f"\n{'='*50}")
    print(f"✅ 繁體版本完成！")
    print(f"   書卷數: {output_data['total_books']}")
    print(f"   經文總數: {output_data['total_verses']}")
    print(f"   輸出檔案: {output_file}")
    
    # 自動生成簡體版本
    simplified_data = convert_to_simplified(output_data)
    
    # 產生簡體版本檔名
    simplified_file = output_file.replace('.json', '_simplified.json')
    
    with open(simplified_file, 'w', encoding='utf-8') as f:
        json.dump(simplified_data, f, ensure_ascii=False, indent=2)
    
    print(f"   簡體檔案: {simplified_file}")
    print(f"{'='*50}")
    print(f"\n📝 版權聲明：")
    print(f"   經文由台灣聖經公會提供")
    print(f"   不超過500節經文的使用權無須預先獲得批准")
    print(f"   但請註明版權所屬")
    print(f"\n🚀 下一步：")
    print(f"   # 匯入繁體版本")
    print(f"   go run cmd/importer/main.go -file {output_file}")
    print(f"   # 匯入簡體版本")
    print(f"   go run cmd/importer/main.go -file {simplified_file}")


def main():
    import argparse
    
    parser = argparse.ArgumentParser(
        description='從信望愛聖經網站抓取新標點和合本神版',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
範例用法:
  # 抓取所有66卷書（需要較長時間）
  %(prog)s -o bible_full.json
  
  # 只抓取創世記（測試用）
  %(prog)s -o bible_test.json -s 1 -e 1
  
  # 抓取舊約（1-39卷）
  %(prog)s -o bible_old_testament.json -s 1 -e 39
  
  # 抓取新約（40-66卷）
  %(prog)s -o bible_new_testament.json -s 40 -e 66
  
注意：
  - 完整66卷約需30-60分鐘
  - 請確保網路連接穩定
  - 遵守版權聲明
        """
    )
    
    parser.add_argument('-o', '--output', 
                       default='bible_cunp89.json',
                       help='輸出檔案名稱（預設: bible_cunp89.json）')
    parser.add_argument('-s', '--start', 
                       type=int, 
                       default=1,
                       help='從第幾卷開始（1-66，預設: 1）')
    parser.add_argument('-e', '--end', 
                       type=int, 
                       default=66,
                       help='到第幾卷結束（1-66，預設: 66）')
    parser.add_argument('--test', 
                       action='store_true',
                       help='測試模式：只抓取創世記第1章')
    
    args = parser.parse_args()
    
    # 驗證參數
    if args.start < 1 or args.start > 66:
        print("❌ 錯誤：起始卷數必須在 1-66 之間", file=sys.stderr)
        sys.exit(1)
    
    if args.end < 1 or args.end > 66:
        print("❌ 錯誤：結束卷數必須在 1-66 之間", file=sys.stderr)
        sys.exit(1)
    
    if args.start > args.end:
        print("❌ 錯誤：起始卷數不能大於結束卷數", file=sys.stderr)
        sys.exit(1)
    
    # 測試模式
    if args.test:
        print("🧪 測試模式：只抓取創世記第1章\n")
        verses = fetch_chapter("創", 1)
        test_data = {
            "version": {
                "code": "CUNP89",
                "name": "新標點和合本神版"
            },
            "books": [
                {
                    "number": 1,
                    "name": "創世記",
                    "abbreviation": "創",
                    "chapters": [
                        {
                            "number": 1,
                            "verses": verses
                        }
                    ]
                }
            ],
            "total_books": 1,
            "total_verses": len(verses)
        }
        
        # 儲存繁體版本
        with open(args.output, 'w', encoding='utf-8') as f:
            json.dump(test_data, f, ensure_ascii=False, indent=2)
        
        print(f"\n✅ 繁體測試資料已儲存: {args.output}")
        print(f"   經文數: {len(verses)}")
        
        # 自動生成簡體版本
        simplified_data = convert_to_simplified(test_data)
        simplified_file = args.output.replace('.json', '_simplified.json')
        
        with open(simplified_file, 'w', encoding='utf-8') as f:
            json.dump(simplified_data, f, ensure_ascii=False, indent=2)
        
        print(f"   簡體檔案: {simplified_file}")
        return
    
    # 正式抓取
    try:
        fetch_all_books(args.output, args.start, args.end)
    except KeyboardInterrupt:
        print("\n\n⚠️  使用者中斷，正在儲存已獲取的資料...")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ 發生錯誤: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == '__main__':
    # 檢查依賴
    try:
        import requests
        from bs4 import BeautifulSoup
        import opencc
    except ImportError:
        print("❌ 缺少依賴套件，請先安裝：")
        print("   pip install requests beautifulsoup4 opencc-python-reimplemented")
        sys.exit(1)
    
    main()
