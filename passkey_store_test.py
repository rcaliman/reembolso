"""Testes da persistência de passkeys; a verificação WebAuthn fica nos testes Go."""
import base64
from contextlib import closing
import hashlib
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import time
import unittest

SCRIPT = Path(__file__).with_name('store.py')

class PasskeyStoreTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='reembolso-passkey-', dir='/tmp')
        self.addCleanup(self.temp.cleanup)
        self.db = str(Path(self.temp.name) / 'test.sqlite')
        self.call('init', {'admin_password':'admin-test-password'})
        self.login = self.call('login', {'login':'admin','senha':'admin-test-password'})
        self.session = hashlib.sha256(self.login['token'].encode()).hexdigest()
        self.user = self.call('passkey_user', {'id':1,'rp_id':'localhost','create':True,'senha':'admin-test-password'})

    def call(self, action, data=None):
        process = subprocess.run([sys.executable,str(SCRIPT),self.db,action],input=json.dumps(data),text=True,capture_output=True,check=True)
        return json.loads(process.stdout)

    def save(self, **overrides):
        data = {'id':1,'rp_id':'localhost','cred_id':'test-id','credential':{'id':'aWQ=','authenticator':{'signCount':0}},'nome':'Meu celular','session':self.session}
        data.update(overrides)
        return self.call('passkey_save',data)

    def login_key(self, version=0):
        return self.call('passkey_login', {'id':1,'rp_id':'localhost','cred_id':'test-id','credential':{'id':'aWQ=','authenticator':{'signCount':version+1},'flags':{'backupState':True}},'version':version})

    def test_password_required_for_enrollment_and_valid_session_at_commit(self):
        result = self.call('passkey_user',{'id':1,'rp_id':'localhost','create':True,'senha':'errada'})
        self.assertEqual(result['status'],400)
        self.assertEqual(self.save(session='invalida')['status'],401)
        self.assertEqual(self.call('passkey_list',{'id':1}),[])
        self.assertTrue(self.save()['ok'])
        self.assertEqual(self.save()['status'],409)
        self.assertEqual(len(base64.b64decode(self.user['handle'])),32)
        self.assertEqual(self.user['handle'],self.call('passkey_user',{'id':1,'rp_id':'localhost'})['handle'])

    def test_login_updates_entire_credential_and_detects_concurrent_updates(self):
        self.save()
        result = self.login_key()
        self.assertEqual(result['usuario']['id'],1)
        self.assertEqual(self.call('session',{'token':result['token']})['id'],1)
        loaded = self.call('passkey_user',{'id':1,'rp_id':'localhost'})
        self.assertEqual(loaded['versions']['test-id'],1)
        self.assertTrue(loaded['credentials'][0]['flags']['backupState'])
        self.assertEqual(self.login_key()['status'],401)
        self.assertIn('token',self.login_key(1))

    def test_lookup_requires_credential_user_handle_and_rp(self):
        self.save()
        data = {'handle':self.user['handle'],'rp_id':'localhost','cred_id':'test-id'}
        self.assertEqual(self.call('passkey_lookup',data)['id'],1)
        for key in data:
            invalid=dict(data);invalid[key]='outra-conta'
            self.assertIsNone(self.call('passkey_lookup',invalid))

    def test_deletion_requires_owner_and_password_and_revokes_key_sessions(self):
        self.save();session = self.login_key()['token']
        key = self.call('passkey_list',{'id':1})[0]
        self.assertNotIn('dados',key)
        self.assertNotIn('cred_id',key)
        self.assertEqual(self.call('passkey_delete',{'id':1,'key_id':key['id'],'senha':'errada'})['status'],400)
        self.call('create_user',{'login':'outra','senha':'outra-senha','perfil':'normal'})
        self.assertEqual(self.call('passkey_delete',{'id':2,'key_id':key['id'],'senha':'outra-senha'})['status'],404)
        self.assertTrue(self.call('passkey_delete',{'id':1,'key_id':key['id'],'senha':'admin-test-password'})['ok'])
        self.assertIsNone(self.call('session',{'token':session}))
        self.assertIsNotNone(self.call('session',{'token':self.login['token']}))
        self.assertEqual(self.login_key(1)['status'],401)

    def test_challenges_are_single_use_expire_and_bind_to_ceremony_session_origin(self):
        data={'token':'desafio','tipo':'register','origem':'http://localhost','payload':{'session':{'challenge':'nonce'}},'id':1,'sessao':self.session}
        self.assertTrue(self.call('passkey_challenge_save',data)['ok'])
        take={k:v for k,v in data.items() if k!='payload'}
        for key in ('tipo','origem','sessao','id'):
            wrong=dict(take);wrong[key]=2 if key=='id' else 'outro'
            self.assertIsNone(self.call('passkey_challenge_take',wrong))
        self.assertEqual(self.call('passkey_challenge_take',take),data['payload'])
        self.assertIsNone(self.call('passkey_challenge_take',take))
        self.call('passkey_challenge_save',data)
        with closing(sqlite3.connect(self.db)) as con, con:
            con.execute('UPDATE passkey_desafios SET expira=?',(int(time.time())-1,))
        self.assertIsNone(self.call('passkey_challenge_take',take))

    def test_password_change_revokes_pending_enrollment_and_sessions(self):
        data={'token':'desafio','tipo':'register','origem':'http://localhost','payload':{},'id':1,'sessao':self.session}
        self.call('passkey_challenge_save',data)
        self.assertTrue(self.call('password',{'id':1,'atual':'admin-test-password','nova':'nova-senha'})['ok'])
        self.assertIsNone(self.call('passkey_challenge_take',data))
        self.assertEqual(self.save()['status'],401)

    def test_limits_bound_credentials_and_anonymous_challenges(self):
        with closing(sqlite3.connect(self.db)) as con, con:
            con.executemany('INSERT INTO passkeys(usuario_id,rp_id,cred_id,dados,nome) VALUES (1,?,?,?,?)',
                [('localhost',str(i),'{}','Aparelho') for i in range(32)])
            con.executemany('INSERT INTO passkey_desafios(token,tipo,origem,dados,usuario_id,sessao,expira) VALUES (?,?,?,?,?,?,?)',
                [(str(i),'login','http://localhost','{}',None,'',int(time.time())+300) for i in range(1024)])
        self.assertEqual(self.save()['status'],400)
        self.assertEqual(self.call('passkey_user',{'id':1,'rp_id':'localhost','create':True,'senha':'admin-test-password'})['status'],400)
        self.assertEqual(self.call('passkey_challenge_save',{'token':'novo','tipo':'login','origem':'http://localhost','payload':{}})['status'],429)

    def test_anonymous_capacity_does_not_block_registration(self):
        with closing(sqlite3.connect(self.db)) as con, con:
            con.executemany('INSERT INTO passkey_desafios(token,tipo,origem,dados,usuario_id,sessao,expira) VALUES (?,?,?,?,?,?,?)',
                [(str(i),'login','http://localhost','{}',None,'',int(time.time())+300) for i in range(1024)])
        self.assertTrue(self.call('passkey_challenge_save',{'token':'registration','tipo':'register',
            'origem':'http://localhost','id':1,'sessao':self.session,'payload':{}})['ok'])

    def test_challenge_quota_is_per_client_and_replacement_does_not_accumulate(self):
        for i in range(16):
            self.assertTrue(self.call('passkey_challenge_save',{'token':str(i),'tipo':'login',
                'origem':'http://localhost','cliente':'client-a','payload':{}})['ok'])
        data={'token':'overflow','tipo':'login','origem':'http://localhost','cliente':'client-a','payload':{}}
        self.assertEqual(self.call('passkey_challenge_save',data)['status'],429)
        self.assertTrue(self.call('passkey_challenge_save',dict(data,previous='0'))['ok'])
        self.assertTrue(self.call('passkey_challenge_save',dict(data,token='other',cliente='client-b'))['ok'])

    def test_old_schema_migration_preserves_sessions(self):
        other = str(Path(self.temp.name) / 'legacy.sqlite')
        with closing(sqlite3.connect(other)) as con, con:
            con.execute('CREATE TABLE sessoes(token TEXT PRIMARY KEY, usuario_id INTEGER NOT NULL, expira INTEGER NOT NULL)')
            con.execute('INSERT INTO sessoes VALUES (?,?,?)',('old',1,int(time.time())+600))
        self.db=other
        self.call('init', {'admin_password':'admin-test-password'});self.call('init', {'admin_password':'admin-test-password'})
        with closing(sqlite3.connect(other)) as con, con:
            self.assertEqual(con.execute('SELECT token FROM sessoes').fetchone(),('old',))
            self.assertEqual(len(list(con.execute('PRAGMA table_info(sessoes)'))),3)

    def test_user_deletion_removes_handles_credentials_and_pending_challenges(self):
        self.call('create_user',{'login':'normal','senha':'senha-normal','perfil':'normal'})
        account=self.call('login',{'login':'normal','senha':'senha-normal'})
        session=hashlib.sha256(account['token'].encode()).hexdigest()
        user=self.call('passkey_user',{'id':2,'rp_id':'localhost','create':True,'senha':'senha-normal'})
        self.save(id=2,session=session)
        self.call('passkey_challenge_save',{'token':'pending','tipo':'register','origem':'http://localhost','id':2,'sessao':session,'payload':{}})
        self.assertTrue(self.call('delete_user',{'id':2,'ator':1})['ok'])
        self.assertEqual(self.call('passkey_list',{'id':2}),[])
        self.assertIsNone(self.call('passkey_lookup',{'handle':user['handle'],'rp_id':'localhost','cred_id':'test-id'}))
        with closing(sqlite3.connect(self.db)) as con, con:
            self.assertEqual(con.execute('SELECT COUNT(*) FROM passkey_desafios WHERE usuario_id=2').fetchone()[0],0)

if __name__=='__main__':
    unittest.main()
