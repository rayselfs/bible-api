FROM python:3.9-slim

WORKDIR /app

# Install system dependencies for psycopg2 (if using source build, but binary is usually fine)
# minimal requirements
RUN pip install --no-cache-dir sentence-transformers psycopg2-binary toml

COPY generate_vectors.py .

CMD ["python", "generate_vectors.py"]
