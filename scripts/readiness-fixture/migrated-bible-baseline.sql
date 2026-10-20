-- Synthetic already-migrated baseline for isolated handoff tests only.
-- Historical virgin migrations use current GORM models and cannot be used as this baseline.
CREATE EXTENSION IF NOT EXISTS vector;
CREATE TABLE IF NOT EXISTS versions(id bigserial PRIMARY KEY,code varchar(20) UNIQUE NOT NULL,name varchar(100) NOT NULL,locale varchar(20),updated_at timestamptz DEFAULT now());
CREATE TABLE IF NOT EXISTS books(id bigserial PRIMARY KEY,version_id bigint NOT NULL REFERENCES versions(id) ON DELETE CASCADE,number integer NOT NULL,name varchar(100) NOT NULL,abbreviation varchar(20));
CREATE TABLE IF NOT EXISTS chapters(id bigserial PRIMARY KEY,book_id bigint NOT NULL REFERENCES books(id) ON DELETE CASCADE,number integer NOT NULL);
CREATE TABLE IF NOT EXISTS verses(id bigserial PRIMARY KEY,chapter_id bigint NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,number integer NOT NULL,text text NOT NULL);
CREATE TABLE IF NOT EXISTS bible_vectors(id serial PRIMARY KEY,verse_id bigint NOT NULL REFERENCES verses(id) ON DELETE CASCADE,embedding vector(384),updated_at timestamptz DEFAULT now());
CREATE TABLE IF NOT EXISTS vector_update_logs(id serial PRIMARY KEY,source varchar(50) NOT NULL,reference_id varchar(100),status varchar(20) DEFAULT 'pending',created_at timestamptz DEFAULT now(),updated_at timestamptz DEFAULT now());
CREATE TABLE IF NOT EXISTS migrations(id varchar(255) PRIMARY KEY);
INSERT INTO migrations(id) VALUES ('202508302145_INITIAL_SCHEMA'),('202511261800_ADD_HYBRID_SEARCH'),('202512161200_ADD_UPDATED_AT_VERSIONS'),('202512301230_CHANGE_VECTOR_DIMENSION'),('202512301330_ADD_VECTOR_UPDATE_LOGS'),('202504251200_ADD_LOCALE_TO_VERSIONS') ON CONFLICT DO NOTHING;
INSERT INTO versions VALUES(1,'KJV','Handoff fixture','en',now()) ON CONFLICT DO NOTHING;
INSERT INTO books VALUES(1,1,1,'Fixture','F') ON CONFLICT DO NOTHING;
INSERT INTO chapters VALUES(1,1,1) ON CONFLICT DO NOTHING;
INSERT INTO verses VALUES(1,1,1,'Handoff sentinel') ON CONFLICT DO NOTHING;
INSERT INTO bible_vectors(id,verse_id,embedding) VALUES(1,1,array_fill(0::real,ARRAY[384])::vector) ON CONFLICT DO NOTHING;
