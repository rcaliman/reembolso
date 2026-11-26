"""Regressões das migrações e da autenticação por senha."""
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('store.py').resolve()

class StoreTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir='/tmp')
        self.addCleanup(self.temp.cleanup)
        self.db = str(Path(self.temp.name) / 'test.sqlite')

    def call(self, action, data=None):
        result = subprocess.run([sys.executable, str(SCRIPT), self.db, action],
                                input=json.dumps(data), text=True, capture_output=True, check=True)
        return json.loads(result.stdout)

    def test_bootstrap_generates_unique_password_and_preserves_existing_account(self):
        initial = self.call('init')
        password = initial['senha_inicial']
        self.assertGreaterEqual(len(password), 24)
        self.assertEqual(self.call('login', {'login':'admin','senha':'admin'})['status'], 401)
        self.assertIn('token', self.call('login', {'login':'admin','senha':password}))
        self.assertNotIn('senha_inicial', self.call('init', {'admin_password':'replacement-password'}))
        self.assertIn('token', self.call('login', {'login':'admin','senha':password}))

    def test_login_rechecks_user_and_password_after_slow_hash(self):
        self.call('init', {'admin_password':'original-password'})
        # Uma segunda conexão altera o usuário exatamente durante o cálculo do hash.
        harness = '''import hashlib,json,runpy,sqlite3,sys
script,db,mutation=sys.argv[1:]
original=hashlib.pbkdf2_hmac
def concurrent_change(*args,**kwargs):
    digest=original(*args,**kwargs)
    with sqlite3.connect(db) as other:
        other.executescript(mutation)
    return digest
hashlib.pbkdf2_hmac=concurrent_change
sys.argv=[script,db,'login']
runpy.run_path(script,run_name='__main__')
'''
        for mutation in [
            "DELETE FROM usuarios WHERE id=1; INSERT INTO usuarios(id,login,senha,perfil) VALUES(1,'substituto','different','admin');",
            "UPDATE usuarios SET senha='changed' WHERE id=1;",
        ]:
            with self.subTest(mutation=mutation):
                # Restaura uma senha válida para cada cenário independente.
                with sqlite3.connect(self.db) as con:
                    con.execute('DELETE FROM sessoes')
                    con.execute('DELETE FROM usuarios')
                    con.execute("DELETE FROM configuracao WHERE chave='auth_inicializada'")
                self.call('init', {'admin_password':'original-password'})
                result = subprocess.run([sys.executable,'-c',harness,str(SCRIPT),self.db,mutation],
                    input=json.dumps({'login':'admin','senha':'original-password'}),
                    text=True,capture_output=True,check=True)
                self.assertEqual(json.loads(result.stdout)['status'],401)
                with sqlite3.connect(self.db) as con:
                    self.assertEqual(con.execute('SELECT COUNT(*) FROM sessoes').fetchone()[0],0)

if __name__ == '__main__':
    unittest.main()
