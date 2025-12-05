#!/usr/bin/env python3
"""
JSON 檔案繁簡轉換工具
將繁體 JSON 檔案轉換為簡體版本
"""

import json
import os
import sys
import argparse
from pathlib import Path
from typing import Dict, Any
import opencc

class JSONSimplifiedConverter:
    def __init__(self):
        """初始化轉換器"""
        print("🔄 初始化 OpenCC 轉換器...")
        self.converter = opencc.OpenCC('t2s')  # 繁體轉簡體
        print("✅ 轉換器初始化完成")
    
    def convert_version_info(self, version_info: Dict[str, Any]) -> Dict[str, Any]:
        """轉換版本資訊"""
        return {
            "code": version_info["code"] + "-SC",  # 加上 -SC 後綴
            "name": self.converter.convert(version_info["name"]) + " (简体)"
        }
    
    def convert_book(self, book: Dict[str, Any]) -> Dict[str, Any]:
        """轉換書卷資訊"""
        return {
            "number": book["number"],
            "name": self.converter.convert(book["name"]),
            "abbreviation": self.converter.convert(book["abbreviation"]),
            "chapters": [self.convert_chapter(chapter) for chapter in book["chapters"]]
        }
    
    def convert_chapter(self, chapter: Dict[str, Any]) -> Dict[str, Any]:
        """轉換章節資訊"""
        return {
            "number": chapter["number"],
            "verses": [self.convert_verse(verse) for verse in chapter["verses"]]
        }
    
    def convert_verse(self, verse: Dict[str, Any]) -> Dict[str, Any]:
        """轉換經文內容"""
        return {
            "number": verse["number"],
            "text": self.converter.convert(verse["text"])
        }
    
    def convert_bible_data(self, data: Dict[str, Any]) -> Dict[str, Any]:
        """轉換整個聖經資料"""
        print("🔄 開始轉換聖經資料...")
        
        # 轉換版本資訊
        version_info = self.convert_version_info(data["version"])
        print(f"   版本: {data['version']['name']} → {version_info['name']}")
        
        # 轉換書卷
        books = []
        total_books = len(data["books"])
        
        for i, book in enumerate(data["books"], 1):
            print(f"   轉換書卷 {i}/{total_books}: {book['name']}")
            converted_book = self.convert_book(book)
            books.append(converted_book)
        
        # 構建轉換後的資料
        converted_data = {
            "version": version_info,
            "books": books,
            "total_books": data["total_books"],
            "total_verses": data["total_verses"]
        }
        
        print("✅ 轉換完成")
        return converted_data
    
    def convert_file(self, input_file: str, output_file: str = None):
        """轉換單個檔案"""
        print(f"\n{'='*60}")
        print(f"轉換檔案: {input_file}")
        print(f"{'='*60}")
        
        # 檢查輸入檔案
        if not os.path.exists(input_file):
            print(f"❌ 輸入檔案不存在: {input_file}")
            return False
        
        # 生成輸出檔案名
        if output_file is None:
            input_path = Path(input_file)
            output_file = str(input_path.parent / f"{input_path.stem}_simplified{input_path.suffix}")
        
        try:
            # 載入 JSON 檔案
            print("📖 載入 JSON 檔案...")
            with open(input_file, 'r', encoding='utf-8') as f:
                data = json.load(f)
            
            print(f"✅ 載入成功")
            print(f"   版本: {data['version']['name']}")
            print(f"   書卷數: {data['total_books']}")
            print(f"   經文數: {data['total_verses']}")
            
            # 轉換資料
            converted_data = self.convert_bible_data(data)
            
            # 保存轉換後的檔案
            print(f"💾 保存轉換後的檔案: {output_file}")
            with open(output_file, 'w', encoding='utf-8') as f:
                json.dump(converted_data, f, ensure_ascii=False, indent=2)
            
            # 檢查檔案大小
            input_size = os.path.getsize(input_file) / 1024 / 1024
            output_size = os.path.getsize(output_file) / 1024 / 1024
            
            print(f"✅ 轉換完成")
            print(f"   輸入檔案: {input_size:.1f} MB")
            print(f"   輸出檔案: {output_size:.1f} MB")
            print(f"   檔案路徑: {output_file}")
            
            return True
            
        except json.JSONDecodeError as e:
            print(f"❌ JSON 解析錯誤: {e}")
            return False
        except Exception as e:
            print(f"❌ 轉換失敗: {e}")
            return False

def main():
    """主函數"""
    parser = argparse.ArgumentParser(description='JSON 檔案繁簡轉換工具')
    parser.add_argument('--input', '-i', required=True, help='輸入的 JSON 檔案路徑')
    parser.add_argument('--output', '-o', help='輸出的 JSON 檔案路徑 (可選)')
    parser.add_argument('--batch', '-b', nargs='+', help='批量轉換多個檔案')
    parser.add_argument('--preview', '-p', action='store_true', help='預覽轉換效果')
    
    args = parser.parse_args()
    
    # 檢查依賴
    try:
        import opencc
    except ImportError:
        print("❌ 缺少依賴套件，請先安裝：")
        print("   pip install opencc-python-reimplemented")
        sys.exit(1)
    
    # 初始化轉換器
    converter = JSONSimplifiedConverter()
    
    # 批量轉換
    if args.batch:
        print(f"🔄 批量轉換 {len(args.batch)} 個檔案...")
        success_count = 0
        
        for input_file in args.batch:
            if converter.convert_file(input_file):
                success_count += 1
        
        print(f"\n🎉 批量轉換完成！")
        print(f"   成功: {success_count}/{len(args.batch)}")
        return
    
    # 單檔案轉換
    if args.preview:
        # 預覽模式
        print("🔍 預覽轉換效果...")
        try:
            with open(args.input, 'r', encoding='utf-8') as f:
                data = json.load(f)
            
            # 轉換版本資訊和第一本書的第一章第一節
            version_info = converter.convert_version_info(data["version"])
            first_book = data["books"][0]
            first_chapter = first_book["chapters"][0]
            first_verse = first_chapter["verses"][0]
            
            print(f"\n📖 轉換預覽:")
            print(f"   版本: {data['version']['name']} → {version_info['name']}")
            print(f"   書卷: {first_book['name']} → {converter.converter.convert(first_book['name'])}")
            print(f"   經文: {first_verse['text']} → {converter.converter.convert(first_verse['text'])}")
            
        except Exception as e:
            print(f"❌ 預覽失敗: {e}")
    else:
        # 正常轉換
        if converter.convert_file(args.input, args.output):
            print(f"\n🎉 轉換成功！")
        else:
            print(f"\n❌ 轉換失敗！")
            sys.exit(1)

if __name__ == "__main__":
    main()
