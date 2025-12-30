
import os
import sys
import psycopg2
from psycopg2.extras import execute_values
from sentence_transformers import SentenceTransformer

# Configuration
DB_HOST = os.getenv("DB_HOST", "localhost")
DB_PORT = os.getenv("DB_PORT", "5432")
DB_USER = os.getenv("DB_USER", "postgres")
DB_PASSWORD = os.getenv("DB_PASSWORD", "postgres")
DB_NAME = os.getenv("DB_NAME", "bible_db")
DB_SSLMODE = os.getenv("DB_SSLMODE", "disable")
MODEL_NAME = "paraphrase-multilingual-MiniLM-L12-v2"
BATCH_SIZE = 100

def get_db_connection():
    try:
        conn = psycopg2.connect(
            host=DB_HOST,
            port=DB_PORT,
            user=DB_USER,
            password=DB_PASSWORD,
            dbname=DB_NAME,
            sslmode=DB_SSLMODE
        )
        return conn
    except Exception as e:
        print(f"Error connecting to database: {e}")
        sys.exit(1)

def migrate_vectors():
    print(f"Loading model: {MODEL_NAME}...")
    model = SentenceTransformer(MODEL_NAME)
    
    conn = get_db_connection()
    cur = conn.cursor()

    try:
        # 1. Truncate existing vectors
        print("Truncating 'bible_vectors' table...")
        cur.execute("TRUNCATE TABLE bible_vectors RESTART IDENTITY")
        conn.commit()

        # 2. Fetch all verses
        print("Fetching all verses...")
        cur.execute("SELECT id, text FROM verses ORDER BY id")
        verses = cur.fetchall()
        
        total_verses = len(verses)
        print(f"Found {total_verses} verses. Starting processing...")

        # 3. Process in batches
        updated_count = 0
        
        for i in range(0, total_verses, BATCH_SIZE):
            batch = verses[i:i + BATCH_SIZE]
            texts = [v[1] for v in batch]
            ids = [v[0] for v in batch]
            
            # Generate embeddings
            embeddings = model.encode(texts)
            
            # Prepare data
            insert_data = []
            for j, embedding in enumerate(embeddings):
                # Format as vector string for pgvector: '[0.1,0.2,...]'
                vector_str = str(embedding.tolist())
                # (verse_id, embedding)
                insert_data.append((ids[j], vector_str))
            
            # Bulk Insert
            execute_values(
                cur,
                "INSERT INTO bible_vectors (verse_id, embedding) VALUES %s",
                insert_data,
                template=None,
                page_size=100
            )
            
            updated_count += len(batch)
            print(f"Progress: {updated_count}/{total_verses} ({(updated_count/total_verses)*100:.1f}%)")
            
            conn.commit()

        print("Vector migration completed successfully!")

    except Exception as e:
        print(f"An error occurred: {e}")
        conn.rollback()
    finally:
        cur.close()
        conn.close()

if __name__ == "__main__":
    import argparse
    
    # Check for library availability
    try:
        import sentence_transformers
        import psycopg2
    except ImportError as e:
        print("Missing dependencies. Please run:")
        print("pip install sentence-transformers psycopg2-binary")
        sys.exit(1)

    parser = argparse.ArgumentParser(description='Generate Bible Vectors')
    parser.add_argument('--force', action='store_true', help='Force regeneration regardless of pending logs')
    parser.add_argument('--check', action='store_true', help='Check for pending logs and run if needed (Default behavior)')
    args = parser.parse_args()

    conn = get_db_connection()
    cur = conn.cursor()

    try:
        should_run = False
        
        if args.force:
            print("Force mode enabled. Starting regeneration...")
            should_run = True
        else:
            # Check for pending logs
            cur.execute("SELECT COUNT(*) FROM vector_update_logs WHERE status = 'pending'")
            count = cur.fetchone()[0]
            if count > 0:
                print(f"Found {count} pending vector update logs. Starting regeneration...")
                should_run = True
                
                # Update status to processing
                cur.execute("UPDATE vector_update_logs SET status = 'processing', updated_at = NOW() WHERE status = 'pending'")
                conn.commit()
            else:
                print("No pending vector updates found.")
        
        if should_run:
            migrate_vectors()
            
            # If we ran based on logs (or even force), we should mark processing logs as completed
            # (If force ran, maybe we also want to clear pending logs? Yes, assume so)
            cur.execute("UPDATE vector_update_logs SET status = 'completed', updated_at = NOW() WHERE status IN ('pending', 'processing')")
            conn.commit()
            print("Updated log status to 'completed'.")

    except Exception as e:
        print(f"Error during execution: {e}")
        # If possible, mark logs as failed? Or leave as processing/pending?
        conn.rollback()
    finally:
        cur.close()
        conn.close()
