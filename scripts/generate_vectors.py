
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
MODEL_NAME = "paraphrase-multilingual-MiniLM-L12-v2"
BATCH_SIZE = 100

def get_db_connection():
    try:
        conn = psycopg2.connect(
            host=DB_HOST,
            port=DB_PORT,
            user=DB_USER,
            password=DB_PASSWORD,
            dbname=DB_NAME
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
        # 1. Fetch all verses
        print("Fetching all verses...")
        cur.execute("SELECT id, text_content FROM verses ORDER BY id")
        verses = cur.fetchall()
        
        total_verses = len(verses)
        print(f"Found {total_verses} verses. Starting processing...")

        # 2. Process in batches
        updated_count = 0
        
        for i in range(0, total_verses, BATCH_SIZE):
            batch = verses[i:i + BATCH_SIZE]
            texts = [v[1] for v in batch]
            ids = [v[0] for v in batch]
            
            # Generate embeddings
            embeddings = model.encode(texts)
            
            # Prepare update data
            update_data = []
            for j, embedding in enumerate(embeddings):
                # Format as vector string for pgvector: '[0.1,0.2,...]'
                vector_str = str(embedding.tolist())
                update_data.append((vector_str, ids[j]))
            
            # Bulk update
            # Note: We assume the column is named 'embedding'. 
            # If migrating, we might want to ensure the column type is correct first (vector(384)).
            # This script assumes the DB schema has already been altered to vector(384).
            execute_values(
                cur,
                "UPDATE verses SET embedding = data.v::vector FROM (VALUES %s) AS data(v, i) WHERE verses.id = data.i",
                update_data,
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
    # Check for library availability
    try:
        import sentence_transformers
        import psycopg2
    except ImportError as e:
        print("Missing dependencies. Please run:")
        print("pip install sentence-transformers psycopg2-binary")
        sys.exit(1)

    print("WARNING: This script will overwrite vectors in the 'verses' table.")
    print("Ensure you have altered the column type to vector(384) BEFORE running.")
    confirm = input("Type 'yes' to proceed: ")
    if confirm.lower() == 'yes':
        migrate_vectors()
    else:
        print("Aborted.")
