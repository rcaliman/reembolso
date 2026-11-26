"""Consulta o catálogo embutido sem acessar o banco de pedidos."""
import json
import sqlite3
import sys
import unicodedata
import tempfile
from contextlib import closing, contextmanager
from pathlib import Path


def alphabetical(value):
    return ''.join(c for c in unicodedata.normalize('NFKD', value.casefold()) if not unicodedata.combining(c))


def consultar(con, cidade='', especialidade=''):
    con.row_factory = sqlite3.Row
    providers = []
    for row in con.execute('SELECT * FROM prestadores'):
        p = dict(row)
        for field in ('especialidades', 'areas_atuacao', 'telefones'):
            p[field] = [v for v in json.loads(p[field]) if v]
        providers.append(p)
    cities = sorted({p['cidade'] for p in providers}, key=alphabetical)
    specialties = sorted({s for p in providers for s in p['especialidades']}, key=alphabetical)
    selected = [p for p in providers if (not cidade or p['cidade'] == cidade) and
                (not especialidade or especialidade in p['especialidades'])]
    selected.sort(key=lambda p: (alphabetical(p['cidade']), alphabetical(p['nome']), alphabetical(p['endereco']), p['chave']))
    contacts = {}
    for row in con.execute('SELECT * FROM telefones_web ORDER BY telefone,fonte_url'):
        contacts.setdefault(row['prestador_chave'], []).append(dict(row))
    for p in selected:
        p['telefones_web'] = contacts.get(p['chave'], [])
        p.pop('conteudo_hash', None)
    return {'cidades': cities, 'especialidades': specialties, 'prestadores': selected, 'total': len(selected)}


@contextmanager
def abrir_catalogo(content):
    # deserialize não existe em Python < 3.11 e pode faltar no SQLite instalado.
    with closing(sqlite3.connect(':memory:')) as memory:
        deserialize = getattr(memory, 'deserialize', None)
        if callable(deserialize):
            try:
                deserialize(content)
            except sqlite3.NotSupportedError:
                pass
            else:
                memory.execute('PRAGMA query_only=ON')
                yield memory
                return
    # Cópia privada e descartável, aberta somente para leitura; nunca usa o banco de pedidos.
    with tempfile.TemporaryDirectory(prefix='reembolso-rede-') as directory:
        path = Path(directory) / 'catalogo.sqlite3'
        path.write_bytes(content)
        with closing(sqlite3.connect(path.as_uri() + '?mode=ro', uri=True)) as con:
            con.execute('PRAGMA query_only=ON')
            yield con


if __name__ == '__main__':
    with abrir_catalogo(sys.stdin.buffer.read()) as con:
        print(json.dumps(consultar(con, *sys.argv[1:3]), ensure_ascii=False))
