import sqlite3
import unittest
from rede import consultar, abrir_catalogo
from pathlib import Path
from unittest.mock import patch

class RedeTest(unittest.TestCase):
    def test_catalogue_without_deserialize_is_read_only_and_removed(self):
        original_connect = sqlite3.connect
        class LegacyConnection(sqlite3.Connection):
            deserialize = None
        def legacy_connect(*args, **kwargs):
            kwargs['factory'] = LegacyConnection
            return original_connect(*args, **kwargs)
        content = Path('data/banescaixa.sqlite3').read_bytes()
        with patch('rede.sqlite3.connect', side_effect=legacy_connect):
            with abrir_catalogo(content) as con:
                path = Path(con.execute('PRAGMA database_list').fetchone()[2])
                self.assertTrue(path.exists())
                self.assertEqual(consultar(con, 'COLATINA')['total'], 41)
                with self.assertRaises(sqlite3.OperationalError):
                    con.execute('DELETE FROM prestadores')
            self.assertFalse(path.exists())
            self.assertFalse(path.parent.exists())

    def test_unsupported_deserialize_and_exception_cleanup(self):
        original_connect = sqlite3.connect
        class UnsupportedConnection(sqlite3.Connection):
            def deserialize(self, content):
                raise sqlite3.NotSupportedError('indisponível')
        def unsupported_connect(*args, **kwargs):
            kwargs['factory'] = UnsupportedConnection
            return original_connect(*args, **kwargs)
        with patch('rede.sqlite3.connect', side_effect=unsupported_connect):
            with self.assertRaisesRegex(RuntimeError, 'falha de consulta'):
                with abrir_catalogo(Path('data/banescaixa.sqlite3').read_bytes()) as con:
                    path = Path(con.execute('PRAGMA database_list').fetchone()[2])
                    raise RuntimeError('falha de consulta')
            self.assertFalse(path.parent.exists())

    def test_available_deserialize_matches_fallback(self):
        with abrir_catalogo(Path('data/banescaixa.sqlite3').read_bytes()) as con:
            self.assertEqual(consultar(con, 'COLATINA')['total'], 41)
            with self.assertRaises(sqlite3.OperationalError):
                con.execute('DELETE FROM prestadores')

    def test_accent_order_and_exact_specialty(self):
        c=sqlite3.connect(':memory:')
        self.addCleanup(c.close)
        c.execute('CREATE TABLE prestadores(chave,nome,cidade,especialidades,areas_atuacao,telefones,endereco)')
        c.execute('CREATE TABLE telefones_web(prestador_chave,telefone,fonte_url)')
        for key,name,city,specialty in [('1','Zeca','Águia','CARDIOLOGIA'),('2','Álvaro','Águia','CARDIOLOGIA'),('3','Ana','Vitória','ECOCARDIOGRAFIA'),('4','Ana','Vitória','CARDIOLOGIA')]:
            c.execute('INSERT INTO prestadores VALUES(?,?,?,?,?,?,?)',(key,name,city,'["'+specialty+'"]','[]','[]','Rua A'))
        result=consultar(c,'','CARDIOLOGIA')
        self.assertEqual([(r['cidade'],r['nome']) for r in result['prestadores']],[('Águia','Álvaro'),('Águia','Zeca'),('Vitória','Ana')])
        self.assertEqual(consultar(c,'Águia','CARDIOLOGIA')['total'],2)
        self.assertEqual(consultar(c,'Vitória','ECOCARDIOGRAFIA')['total'],1)
        self.assertEqual(consultar(c,'Águia','CARDIO')['total'],0)

if __name__=='__main__': unittest.main()
