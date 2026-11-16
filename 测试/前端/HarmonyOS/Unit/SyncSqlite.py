"""Private host-only SQLite bridge; never connects to JobsIM or Postgres."""
import json
import os
from pathlib import Path
import sqlite3
import sys

os.umask(0o077)
path = Path(sys.argv[1]).resolve()
db = sqlite3.connect(path, isolation_level=None)
db.execute('PRAGMA journal_mode=WAL')
db.execute('PRAGMA synchronous=FULL')
for line in sys.stdin:
    request = json.loads(line)
    try:
        cursor = db.execute(request['sql'], request.get('values', []))
        rows = cursor.fetchall() if cursor.description else []
        response = {'id': request['id'], 'rows': rows}
    except sqlite3.Error as error:
        response = {'id': request['id'], 'error': str(error)}
    sys.stdout.write(json.dumps(response) + '\n')
    sys.stdout.flush()
db.close()
