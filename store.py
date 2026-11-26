import json, sqlite3, sys, hashlib, hmac, secrets, time, unicodedata, base64
path, action = sys.argv[1:]
data = json.load(sys.stdin)
con = sqlite3.connect(path, timeout=15)
con.row_factory = sqlite3.Row
con.execute("PRAGMA foreign_keys=ON")
def password_hash(password):
    salt = secrets.token_hex(16)
    digest = hashlib.pbkdf2_hmac('sha256', password.encode(), bytes.fromhex(salt), 600000).hex()
    return salt + ':' + digest

def password_ok(password, stored):
    salt, digest = stored.split(':')
    computed = hashlib.pbkdf2_hmac('sha256', password.encode(), bytes.fromhex(salt), 600000).hex()
    return hmac.compare_digest(digest, computed)

def public_user(row):
    return {k:row[k] for k in ('id','login','perfil')}

def issue_session(row, passkey_id=None):
    token = secrets.token_urlsafe(32)
    con.execute('DELETE FROM sessoes WHERE expira<=?',(int(time.time()),))
    digest = hashlib.sha256(token.encode()).hexdigest()
    con.execute('INSERT INTO sessoes(token,usuario_id,expira) VALUES (?,?,?)',(digest,row['id'],int(time.time())+43200))
    if passkey_id is not None:
        con.execute('INSERT INTO passkey_sessoes(token,passkey_id) VALUES (?,?)',(digest,passkey_id))
    return {'token':token,'usuario':public_user(row)}

def passkey_user(row, rp_id):
    handle = con.execute('SELECT handle FROM passkey_usuarios WHERE usuario_id=?',(row['id'],)).fetchone()
    if not handle:
        return None
    credentials = list(con.execute('SELECT cred_id,dados,versao FROM passkeys WHERE usuario_id=? AND rp_id=?',(row['id'],rp_id)))
    return dict(public_user(row),handle=handle['handle'],credentials=[json.loads(p['dados']) for p in credentials],versions={p['cred_id']:p['versao'] for p in credentials})

def snapshot_person(snapshot, role):
    person = snapshot.get(role)
    return {key:person[key] for key in ('nome','inscricao')} if person else None

def error(message, status=400):
    return {'error':message,'status':status}

def person_name(value):
    return ' '.join(''.join(c for c in unicodedata.normalize('NFKD',value.casefold()) if not unicodedata.combining(c)).split())

def person_cpf(value):
    return value.replace('.','').replace('-','').strip()

class PersonConflict(Exception):
    pass

def merge_people(rows):
    # Keep the oldest id; retain the latest nonempty contact details. Historical
    # requests are snapshots and must never be rewritten by this migration.
    rows = sorted(rows,key=lambda row:row['id'])
    primary = dict(rows[0])
    for row in rows[1:]:
        for key in ('email','telefone','email_secundario','cpf'):
            if row[key]:
                primary[key] = row[key]
        con.execute('DELETE FROM pessoas WHERE id=?',(row['id'],))
    con.execute('UPDATE pessoas SET email=?,telefone=?,email_secundario=?,cpf=? WHERE id=?',
        (primary['email'],primary['telefone'],primary['email_secundario'],primary['cpf'],primary['id']))
    return primary

def migrate_person_identity():
    rows = [dict(row) for row in con.execute('SELECT * FROM pessoas ORDER BY id')]
    by_name = {}
    for row in rows:
        by_name.setdefault((row['inscricao'],person_name(row['nome'])),[]).append(row)
    # A legacy blank CPF can join an unambiguous spelling-equivalent identity.
    for group in by_name.values():
        known = {person_cpf(row['cpf']) for row in group if row['cpf']}
        if len(known)<=1:
            cpf = next((row['cpf'] for row in group if row['cpf']),'')
            for row in group:
                if not row['cpf']:
                    row['cpf']=cpf
    groups={}
    for row in rows:
        cpf=person_cpf(row['cpf'])
        key=(row['inscricao'],'cpf',cpf) if cpf else (row['inscricao'],'nome',person_name(row['nome']))
        groups.setdefault(key,[]).append(row)
    for group in groups.values():
        if len(group)>1:
            merge_people(group)
    con.execute("CREATE UNIQUE INDEX IF NOT EXISTS pessoas_inscricao_cpf ON pessoas(inscricao,replace(replace(cpf,'.',''),'-','')) WHERE cpf<>''")

def save_person(person):
    rows=[dict(row) for row in con.execute('SELECT * FROM pessoas WHERE inscricao=? ORDER BY id',(person['inscricao'],))]
    cpf=person_cpf(person.get('cpf',''))
    name=person_name(person['nome'])
    if any(person_name(row['nome'])==name and row['cpf'] and cpf and person_cpf(row['cpf'])!=cpf for row in rows):
        raise PersonConflict('O CPF informado difere do cadastro existente para esta inscrição e nome. Confira os dados antes de salvar.')
    candidates=[row for row in rows if (cpf and person_cpf(row['cpf'])==cpf) or (not row['cpf'] and person_name(row['nome'])==name)]
    if not cpf:
        candidates=[row for row in rows if person_name(row['nome'])==name]
    if candidates:
        current=merge_people(candidates)
        # Keep the canonical spelling to avoid splitting historical filters on
        # case-only edits; a real name correction is stored in future snapshots.
        if person_name(current['nome'])==name:
            person['nome']=current['nome']
        con.execute('UPDATE pessoas SET nome=?,cpf=?,email=?,telefone=?,email_secundario=? WHERE id=?',
            (person['nome'],person.get('cpf') or current['cpf'],person.get('email') or current['email'],
             person.get('telefone') or current['telefone'],person.get('email_secundario',current['email_secundario']),current['id']))
    else:
        con.execute('INSERT INTO pessoas(inscricao,nome,email,telefone,email_secundario,cpf) VALUES (?,?,?,?,?,?)',
            (person['inscricao'],person['nome'],person.get('email',''),person.get('telefone',''),person.get('email_secundario',''),person.get('cpf','')))

if action == 'init':
    con.executescript('''
    PRAGMA journal_mode=WAL;
    CREATE TABLE IF NOT EXISTS pessoas(id INTEGER PRIMARY KEY, inscricao TEXT NOT NULL, nome TEXT NOT NULL, email TEXT NOT NULL, telefone TEXT NOT NULL, UNIQUE(inscricao,nome));
    CREATE TABLE IF NOT EXISTS prestadores(id INTEGER PRIMARY KEY, nome TEXT NOT NULL, documento TEXT NOT NULL, UNIQUE(nome,documento));
    CREATE TABLE IF NOT EXISTS contas(id INTEGER PRIMARY KEY, tipo TEXT NOT NULL, banco TEXT NOT NULL, agencia TEXT NOT NULL, numero TEXT NOT NULL, titular TEXT NOT NULL, cpf TEXT NOT NULL, UNIQUE(tipo,banco,agencia,numero,titular,cpf));
    CREATE TABLE IF NOT EXISTS pedidos(id INTEGER PRIMARY KEY, criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, dados TEXT NOT NULL);
    ''')
    # Migrações preservam os pedidos, cujos dados ficam armazenados como cópias.
    con.executescript("""
    CREATE TABLE IF NOT EXISTS usuarios(id INTEGER PRIMARY KEY, login TEXT NOT NULL COLLATE NOCASE UNIQUE, senha TEXT NOT NULL, perfil TEXT NOT NULL CHECK(perfil IN ('admin','normal')));
    CREATE TABLE IF NOT EXISTS sessoes(token TEXT PRIMARY KEY, usuario_id INTEGER NOT NULL, expira INTEGER NOT NULL);
    CREATE TABLE IF NOT EXISTS configuracao(chave TEXT PRIMARY KEY, valor TEXT NOT NULL);
    """)
    con.executescript("""
    CREATE TABLE IF NOT EXISTS passkey_usuarios(usuario_id INTEGER PRIMARY KEY REFERENCES usuarios(id) ON DELETE CASCADE, handle TEXT NOT NULL UNIQUE);
    CREATE TABLE IF NOT EXISTS passkeys(id INTEGER PRIMARY KEY AUTOINCREMENT, usuario_id INTEGER NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
      rp_id TEXT NOT NULL, cred_id TEXT NOT NULL, dados TEXT NOT NULL, nome TEXT NOT NULL, criado_em TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
      usado_em TEXT, versao INTEGER NOT NULL DEFAULT 0, UNIQUE(rp_id,cred_id));
    CREATE TABLE IF NOT EXISTS passkey_sessoes(token TEXT PRIMARY KEY REFERENCES sessoes(token) ON DELETE CASCADE, passkey_id INTEGER NOT NULL REFERENCES passkeys(id) ON DELETE CASCADE);
    CREATE TABLE IF NOT EXISTS passkey_desafios(token TEXT PRIMARY KEY,tipo TEXT NOT NULL,origem TEXT NOT NULL,dados TEXT NOT NULL,
      usuario_id INTEGER REFERENCES usuarios(id) ON DELETE CASCADE,sessao TEXT NOT NULL,expira INTEGER NOT NULL);
    """)
    con.execute('BEGIN IMMEDIATE')
    # Solicitante/dependente é o papel da pessoa no pedido, não um tipo de cadastro.
    # Mantém o primeiro id e aproveita os contatos não vazios mais recentes.
    if any(row['name']=='tipo' for row in con.execute('PRAGMA table_info(pessoas)')):
        merged = {}
        for row in con.execute('SELECT id,inscricao,nome,email,telefone FROM pessoas ORDER BY id'):
            key = (row['inscricao'],row['nome'])
            if key not in merged:
                merged[key] = dict(row)
            else:
                for field in ('email','telefone'):
                    if row[field]:
                        merged[key][field] = row[field]
        con.execute('CREATE TABLE pessoas_unificadas(id INTEGER PRIMARY KEY, inscricao TEXT NOT NULL, nome TEXT NOT NULL, email TEXT NOT NULL, telefone TEXT NOT NULL, UNIQUE(inscricao,nome))')
        con.executemany('INSERT INTO pessoas_unificadas(id,inscricao,nome,email,telefone) VALUES (:id,:inscricao,:nome,:email,:telefone)',list(merged.values()))
        con.execute('DROP TABLE pessoas')
        con.execute('ALTER TABLE pessoas_unificadas RENAME TO pessoas')
    if not any(row['name']=='cpf' for row in con.execute('PRAGMA table_info(pessoas)')):
        con.execute("ALTER TABLE pessoas ADD COLUMN cpf TEXT NOT NULL DEFAULT ''")
    if not any(row['name']=='email_secundario' for row in con.execute('PRAGMA table_info(pessoas)')):
        con.execute("ALTER TABLE pessoas ADD COLUMN email_secundario TEXT NOT NULL DEFAULT ''")
    migrate_person_identity()
    if not any(row['name']=='descricao' for row in con.execute('PRAGMA table_info(prestadores)')):
        con.execute("ALTER TABLE prestadores ADD COLUMN descricao TEXT NOT NULL DEFAULT ''")
    if not any(row['name']=='reembolsado_centavos' for row in con.execute('PRAGMA table_info(pedidos)')):
        con.execute('ALTER TABLE pedidos ADD COLUMN reembolsado_centavos INTEGER CHECK(reembolsado_centavos >= 0)')
    if not any(row['name']=='observacoes' for row in con.execute('PRAGMA table_info(pedidos)')):
        con.execute("ALTER TABLE pedidos ADD COLUMN observacoes TEXT NOT NULL DEFAULT ''")
    if not any(row['name']=='reembolso_versao' for row in con.execute('PRAGMA table_info(pedidos)')):
        con.execute('ALTER TABLE pedidos ADD COLUMN reembolso_versao INTEGER NOT NULL DEFAULT 0')
    if not any(row['name']=='cliente' for row in con.execute('PRAGMA table_info(passkey_desafios)')):
        con.execute("ALTER TABLE passkey_desafios ADD COLUMN cliente TEXT NOT NULL DEFAULT ''")
    result = {'ok':True}
    if not con.execute("SELECT 1 FROM configuracao WHERE chave='auth_inicializada'").fetchone():
        password = (data or {}).get('admin_password') or secrets.token_urlsafe(24)
        if not 8 <= len(password) <= 128:
            raise ValueError('A senha inicial deve ter entre 8 e 128 caracteres.')
        con.execute("INSERT INTO usuarios(login,senha,perfil) VALUES (?,?,?)", ('admin',password_hash(password),'admin'))
        if not (data or {}).get('admin_password'):
            result['senha_inicial'] = password
        con.execute("INSERT INTO configuracao VALUES ('auth_inicializada','1')")
    # Mantém os identificadores estáveis mesmo após excluir o último pedido.
    con.execute("INSERT OR IGNORE INTO configuracao(chave,valor) SELECT 'pedido_ultimo_id',CAST(COALESCE(MAX(id),0) AS TEXT) FROM pedidos")
    con.execute("CREATE INDEX IF NOT EXISTS pedidos_ano ON pedidos(substr(json_extract(dados,'$.pedido.data'),1,4))")
    con.commit()
elif action == 'login':
    row = con.execute('SELECT * FROM usuarios WHERE login=?',(data['login'],)).fetchone()
    # Também calcula o hash quando o usuário não existe.
    valid = password_ok(data['senha'],row['senha'] if row else '00'*16+':'+'00'*32)
    if row and valid:
        con.execute('BEGIN IMMEDIATE')
        current = con.execute('SELECT * FROM usuarios WHERE id=? AND login=? AND senha=?',
            (row['id'],row['login'],row['senha'])).fetchone()
        result = issue_session(current) if current else error('Usuário ou senha incorretos.',401)
        con.commit()
    else:
        result = error('Usuário ou senha incorretos.',401)
elif action == 'passkey_user':
    row = con.execute('SELECT * FROM usuarios WHERE id=?',(data['id'],)).fetchone()
    if not row:
        result = error('Usuário não encontrado.',404)
    elif data.get('create') and not password_ok(data.get('senha',''),row['senha']):
        result = error('A senha atual está incorreta.')
    elif data.get('create') and con.execute('SELECT COUNT(*) FROM passkeys WHERE usuario_id=?',(data['id'],)).fetchone()[0]>=32:
        result = error('Limite de 32 passkeys atingido. Remova uma antes de cadastrar outra.')
    else:
        if data.get('create'):
            with con:
                con.execute('INSERT OR IGNORE INTO passkey_usuarios(usuario_id,handle) VALUES (?,?)',
                    (row['id'],base64.b64encode(secrets.token_bytes(32)).decode()))
        result = passkey_user(row,data['rp_id'])
elif action == 'passkey_lookup':
    row = con.execute('SELECT u.* FROM usuarios u JOIN passkey_usuarios h ON h.usuario_id=u.id JOIN passkeys p ON p.usuario_id=u.id WHERE h.handle=? AND p.cred_id=? AND p.rp_id=?',
        (data['handle'],data['cred_id'],data['rp_id'])).fetchone()
    result = passkey_user(row,data['rp_id']) if row else None
elif action == 'passkey_list':
    result = [dict(row) for row in con.execute('SELECT id,nome,rp_id,criado_em,usado_em FROM passkeys WHERE usuario_id=? ORDER BY id DESC',(data['id'],))]
elif action == 'passkey_save':
    con.execute('BEGIN IMMEDIATE')
    try:
        if not con.execute('SELECT 1 FROM sessoes WHERE token=? AND usuario_id=? AND expira>?',(data.get('session',''),data['id'],int(time.time()))).fetchone():
            result = error('Sua sessão expirou. Entre novamente.',401)
        elif con.execute('SELECT COUNT(*) FROM passkeys WHERE usuario_id=?',(data['id'],)).fetchone()[0]>=32:
            result = error('Limite de 32 passkeys atingido. Remova uma antes de cadastrar outra.')
        else:
            con.execute('INSERT INTO passkeys(usuario_id,rp_id,cred_id,dados,nome) VALUES (?,?,?,?,?)',
                (data['id'],data['rp_id'],data['cred_id'],json.dumps(data['credential']),data['nome']))
            result = {'ok':True}
        con.commit()
    except sqlite3.IntegrityError:
        con.rollback()
        result = error('Esta passkey já foi cadastrada ou o usuário foi removido.',409)
elif action == 'passkey_delete':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT id FROM passkeys WHERE id=? AND usuario_id=?',(data['key_id'],data['id'])).fetchone()
    owner = con.execute('SELECT senha FROM usuarios WHERE id=?',(data['id'],)).fetchone()
    if not owner or not password_ok(data.get('senha',''),owner['senha']):
        result = error('A senha atual está incorreta.')
    elif row:
        con.execute('DELETE FROM sessoes WHERE token IN (SELECT token FROM passkey_sessoes WHERE passkey_id=?)',(row['id'],))
        con.execute('DELETE FROM passkeys WHERE id=?',(row['id'],))
        result = {'ok':True}
    else:
        result = error('Passkey não encontrada.',404)
    con.commit()
elif action == 'passkey_login':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT p.id,u.login,u.perfil,p.usuario_id FROM passkeys p JOIN usuarios u ON u.id=p.usuario_id WHERE p.usuario_id=? AND p.rp_id=? AND p.cred_id=? AND p.versao=?',
        (data['id'],data['rp_id'],data['cred_id'],data['version'])).fetchone()
    if row:
        con.execute('UPDATE passkeys SET dados=?,versao=versao+1,usado_em=CURRENT_TIMESTAMP WHERE id=?',
            (json.dumps(data['credential']),row['id']))
        result = issue_session({'id':row['usuario_id'],'login':row['login'],'perfil':row['perfil']},row['id'])
    else:
        result = error('Não foi possível confirmar a passkey. Tente novamente.',401)
    con.commit()
elif action == 'passkey_challenge_save':
    con.execute('BEGIN IMMEDIATE')
    con.execute('DELETE FROM passkey_desafios WHERE expira<=? OR token=?',(int(time.time()),data.get('previous','')))
    # Pools independentes: tentativas anônimas não impedem cadastros autenticados.
    client = data.get('cliente','')
    total = con.execute('SELECT COUNT(*) FROM passkey_desafios WHERE tipo=?',(data['tipo'],)).fetchone()[0]
    own = con.execute('SELECT COUNT(*) FROM passkey_desafios WHERE tipo=? AND cliente=?',(data['tipo'],client)).fetchone()[0]
    if total>=1024 or (client and own>=16):
        result = error('Muitas tentativas em andamento. Aguarde alguns minutos.',429)
    else:
        con.execute('INSERT INTO passkey_desafios(token,tipo,origem,dados,usuario_id,sessao,expira,cliente) VALUES (?,?,?,?,?,?,?,?)',
            (data['token'],data['tipo'],data['origem'],json.dumps(data['payload']),data.get('id'),data.get('sessao',''),int(time.time())+300,client))
        result = {'ok':True}
    con.commit()
elif action == 'passkey_challenge_take':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT * FROM passkey_desafios WHERE token=? AND tipo=? AND origem=? AND usuario_id IS ? AND sessao=?',
        (data['token'],data['tipo'],data['origem'],data.get('id'),data.get('sessao',''))).fetchone()
    if row:
        con.execute('DELETE FROM passkey_desafios WHERE token=?',(data['token'],))
    result = json.loads(row['dados']) if row and row['expira']>int(time.time()) else None
    con.commit()
elif action == 'session':
    row = con.execute('SELECT u.* FROM sessoes s JOIN usuarios u ON u.id=s.usuario_id WHERE s.token=? AND s.expira>?',(hashlib.sha256(data['token'].encode()).hexdigest(),int(time.time()))).fetchone()
    result = public_user(row) if row else None
elif action == 'logout':
    with con:
        con.execute('DELETE FROM sessoes WHERE token=?',(hashlib.sha256(data['token'].encode()).hexdigest(),))
    result = {'ok':True}
elif action == 'password':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT * FROM usuarios WHERE id=?',(data['id'],)).fetchone()
    if not row or not password_ok(data['atual'],row['senha']):
        result = error('A senha atual está incorreta.')
    elif not 8 <= len(data['nova']) <= 128:
        result = error('A nova senha deve ter entre 8 e 128 caracteres.')
    else:
        con.execute('UPDATE usuarios SET senha=? WHERE id=?',(password_hash(data['nova']),data['id']))
        con.execute('DELETE FROM sessoes WHERE usuario_id=?',(data['id'],))
        con.execute('DELETE FROM passkey_desafios WHERE usuario_id=?',(data['id'],))
        result = {'ok':True}
    con.commit()
elif action == 'users':
    result = [public_user(row) for row in con.execute('SELECT * FROM usuarios ORDER BY login')]
elif action == 'create_user':
    try:
        with con:
            con.execute('INSERT INTO usuarios(login,senha,perfil) VALUES (?,?,?)',(data['login'],password_hash(data['senha']),data['perfil']))
        result = {'ok':True}
    except sqlite3.IntegrityError:
        result = error('Este nome de usuário já está cadastrado.',409)
elif action == 'delete_user':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT * FROM usuarios WHERE id=?',(data['id'],)).fetchone()
    if data['id'] == data['ator']:
        result = error('Você não pode excluir seu próprio usuário.')
    elif not row:
        result = error('Usuário não encontrado.',404)
    elif row['perfil']=='admin' and con.execute("SELECT COUNT(*) FROM usuarios WHERE perfil='admin'").fetchone()[0]<=1:
        result = error('É necessário manter pelo menos um administrador.')
    else:
        con.execute('DELETE FROM sessoes WHERE usuario_id=?',(data['id'],))
        con.execute('DELETE FROM usuarios WHERE id=?',(data['id'],))
        result = {'ok':True}
    con.commit()
elif action == 'delete_record':
    table = data['tabela']
    if table not in ('pessoas','prestadores','contas'):
        result = error('Cadastro inválido.')
    else:
        with con:
            cur = con.execute('DELETE FROM '+table+' WHERE id=?',(data['id'],))
        result = {'ok':True} if cur.rowcount else error('Cadastro não encontrado.',404)
elif action == 'list':
    def alphabetical(value):
        return ''.join(c for c in unicodedata.normalize('NFKD',value.casefold()) if not unicodedata.combining(c))
    result = {}
    for table in ('pessoas','prestadores','contas'):
        rows = [dict(r) for r in con.execute('SELECT * FROM '+table)]
        name = 'titular' if table == 'contas' else 'nome'
        result[table] = sorted(rows, key=lambda row:(alphabetical(row[name]),row['id']))
elif action == 'save':
    def save(table, values):
        cols=','.join(values)
        con.execute('INSERT OR IGNORE INTO '+table+' ('+cols+') VALUES ('+','.join('?' for _ in values)+')', list(values.values()))
    try:
        with con:
            con.execute('BEGIN IMMEDIATE')
            for kind in ('solicitante','dependente'):
                if kind in data:
                    person = data[kind]
                    save_person(person)
            provider = data['prestador']
            con.execute('''INSERT INTO prestadores(nome,documento,descricao) VALUES (?,?,?)
                ON CONFLICT(nome,documento) DO UPDATE SET descricao=excluded.descricao''',
                (provider['nome'],provider['documento'],provider.get('descricao','')))
            save('contas',data['conta'])
            con.execute("UPDATE configuracao SET valor=CAST(valor AS INTEGER)+1 WHERE chave='pedido_ultimo_id'")
            next_id = int(con.execute("SELECT valor FROM configuracao WHERE chave='pedido_ultimo_id'").fetchone()[0])
            cur=con.execute('INSERT INTO pedidos(id,dados) VALUES (?,?)',(next_id,json.dumps(data,ensure_ascii=False)))
            result={'id':cur.lastrowid}
    except PersonConflict as exc:
        result=error(str(exc),409)
    except sqlite3.IntegrityError:
        result=error('Há um conflito entre os dados informados e os cadastros existentes. Confira inscrição e CPF.',409)
elif action == 'pedido_filters':
    years = [row[0] for row in con.execute("SELECT DISTINCT substr(json_extract(dados,'$.pedido.data'),1,4) AS ano FROM pedidos ORDER BY ano DESC")]
    result = {'anos':years}
    for role in ('solicitante','dependente'):
        query = "SELECT DISTINCT json_extract(dados,'$.%s.nome') AS nome, json_extract(dados,'$.%s.inscricao') AS inscricao FROM pedidos WHERE json_extract(dados,'$.%s') IS NOT NULL" % (role,role,role)
        result[role] = [dict(row) for row in con.execute(query)]
elif action == 'list_pedidos':
    result = []
    query = 'SELECT id,criado_em,dados,observacoes,reembolsado_centavos,reembolso_versao FROM pedidos'
    params = ()
    if data and data.get('ano'):
        query += " WHERE substr(json_extract(dados,'$.pedido.data'),1,4)=?"
        params = (data['ano'],)
    for row in con.execute(query, params):
        snapshot = json.loads(row['dados'])
        result.append({'id':row['id'], 'criado_em':row['criado_em'],
            'data':snapshot['pedido']['data'], 'valor':snapshot['pedido']['valor'],
            'solicitante':snapshot_person(snapshot,'solicitante'), 'dependente':snapshot_person(snapshot,'dependente'),
            'observacoes':row['observacoes'], 'reembolsado_centavos':row['reembolsado_centavos'], 'reembolso_versao':row['reembolso_versao']})
    result.sort(key=lambda p:(p['data'],p['id']), reverse=True)
elif action == 'reembolso':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT observacoes,reembolsado_centavos,reembolso_versao FROM pedidos WHERE id=?',(data['id'],)).fetchone()
    if not row:
        result = error('Pedido não encontrado.',404)
    elif row['reembolso_versao'] != data['versao']:
        result = dict(error('Este reembolso foi alterado em outra sessão. Confira o valor atual antes de salvar.',409), **dict(row))
    else:
        notes = data.get('observacoes', row['observacoes'])
        con.execute('UPDATE pedidos SET observacoes=?,reembolsado_centavos=?,reembolso_versao=reembolso_versao+1 WHERE id=?',(notes,data['centavos'],data['id']))
        result = {'ok':True, 'observacoes':notes, 'reembolsado_centavos':data['centavos'], 'reembolso_versao':row['reembolso_versao']+1}
    con.commit()
elif action == 'delete_pedido':
    con.execute('BEGIN IMMEDIATE')
    row = con.execute('SELECT dados,reembolso_versao FROM pedidos WHERE id=?',(data['id'],)).fetchone()
    if not row:
        result = error('Pedido não encontrado.',404)
    elif row['reembolso_versao'] != data['versao']:
        result = error('O reembolso foi alterado em outra sessão. Recarregue os pedidos antes de excluir.',409)
    else:
        snapshot = json.loads(row['dados'])
        con.execute('DELETE FROM pedidos WHERE id=?',(data['id'],))
        result = {'ok':True,'pastas':[(snapshot.get(key) or {}).get('arquivo','') for key in ('nota','nota2','nota3') if snapshot.get(key)]}
    con.commit()
elif action == 'get':
    row=con.execute('SELECT dados FROM pedidos WHERE id=?',(data['id'],)).fetchone()
    result=json.loads(row['dados']) if row else None
else:
    raise ValueError('Operação inválida')
con.close()
print(json.dumps(result,ensure_ascii=False))
