# Gerador de pedidos de reembolso

Aplicação local em Go para preencher o formulário Banescaixa fornecido nesta pasta. Usa SQLite para armazenar solicitantes, dependentes, prestadores, contas e os pedidos gerados. A página para impressão usa uma imagem de alta resolução do PDF original, preservando sua arte e orientações, com os dados posicionados sobre os campos.

## Executar

Requisitos: Go 1.26 ou superior e Python 3 com o módulo `sqlite3` (incluído nas instalações usuais). Para anexar notas fiscais, requer também Poppler (`pdfinfo`, `pdftoppm` e `pdftotext`), já disponível nesta máquina. Não requer npm nem instalação de pacotes Python. Para compilar, é necessário baixar os módulos Go da biblioteca WebAuthn; o executável pronto já contém essas dependências.

O executável Linux já foi compilado nesta pasta:

```sh
./reembolso
```

Abra **http://127.0.0.1:8080**. Para compilar novamente:

```sh
go mod tidy
go build -buildvcs=false -o reembolso .
./reembolso
```

Se o cache global do Go estiver inacessível, use um cache gravável:

```sh
GOCACHE=/tmp/reembolso-go-cache go build -buildvcs=false -o reembolso .
```

É possível mudar a porta e o banco:

```sh
./reembolso -addr 127.0.0.1:8090 -db data/meu-banco.sqlite
```

Execute a partir desta pasta para usar o banco padrão `data/reembolso.sqlite`. Os arquivos da interface e o script de persistência estão embutidos no executável; Python 3 continua necessário em tempo de execução. Encerre com Ctrl+C.

## Script de inicialização

O script `iniciar.sh`, também incluído no ZIP ARM64, usa `0.0.0.0:5002`.
Execute `./iniciar.sh` para acesso pela rede na porta 5002. Ele muda para a
pasta da aplicação antes de iniciar, usando `data/reembolso.sqlite` mesmo
quando chamado a partir de outra pasta. No Raspberry, a pasta de instalação
pode ser `/home/webserver/reembolso`.

Para escolher outra porta: `./iniciar.sh -addr 0.0.0.0:9090`.
O executável chamado diretamente continua com a porta padrão 8080.

## Gerar os executáveis e o pacote Raspberry

Depois das verificações abaixo, execute:

```sh
GOCACHE=/tmp/reembolso-go-cache python3 scripts/release.py
```

O script gera `reembolso` para Linux AMD64, `reembolso-pi` para Linux ARM64 e `reembolso.zip` para o Raspberry, sempre a partir do mesmo código. Go e os módulos do `go.sum` precisam estar disponíveis. As compilações desabilitam CGO e removem caminhos locais e metadados variáveis; o ZIP usa ordem e datas fixas. Com o mesmo código, versão do Go e dependências, os artefatos são reproduzíveis. `SHA256SUMS` permite conferir os arquivos gerados.

O ZIP contém apenas o executável ARM64, `iniciar.sh`, este README e um manifesto de integridade. Bancos, anexos, certificados e senhas não entram no pacote. Para atualizar uma instalação, substitua o executável e o script de inicialização, preservando as pastas `data` e `anexos`. A primeira execução de uma instalação nova cria seu próprio banco.

## Instalar como aplicativo (PWA)

A aplicação inclui manifesto, ícones para Android/iOS e um service worker. Depois de atualizar o executável e reiniciar o servidor, abra o site no navegador do aparelho:

- **Android / Chromium:** no menu superior da aplicação, escolha **Instalar app**. Quando disponível, a instalação do navegador abre diretamente; caso contrário, são exibidas orientações.
- **iPhone / iPad:** o item **Instalar app** do menu mostra como instalar pelo Safari: **Compartilhar → Adicionar à Tela de Início**. Se aparecer, mantenha **Abrir como App** ativado.
- **Computador:** use a opção de instalação oferecida pelo navegador.

O app instalado abre diretamente em **Pedidos**, em janela própria, e mantém o login com as mesmas sessões do sistema. Uma sessão expirada exige novo login.

**No smartphone, a instalação exige HTTPS com certificado confiável para o nome usado no endereço.** Abrir `http://IP-DO-SERVIDOR:5002` permite usar o site, mas não habilita o service worker nem a instalação PWA completa. A exceção HTTP de `localhost` e `127.0.0.1` vale somente no próprio aparelho em que o servidor está rodando. Requisitos e disponibilidade variam conforme o navegador: [instalação de PWAs (MDN)](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/Guides/Making_PWAs_installable).

O servidor aceita HTTPS diretamente, sem dependências adicionais. Com um certificado confiável e sua chave privada já disponíveis:

```sh
./iniciar.sh -tls-cert /caminho/certificado.pem -tls-key /caminho/chave.pem
```

Acesse então `https://NOME-DO-CERTIFICADO:5002`. Informe as duas opções juntas; ao iniciar sem elas, o servidor continua usando HTTP. Também é possível publicar o servidor por um proxy HTTPS existente. A aplicação não cria certificados nem configura domínios automaticamente.

Quando uma atualização estiver disponível, o aplicativo oferece **Atualizar aplicativo**. Salve os campos em edição antes de clicar: a atualização recarrega a página. Não há recarga forçada no meio do preenchimento.

Sem conexão, uma página pública informa como tentar novamente. O service worker guarda somente essa página e os ícones. Pedidos, dados de pessoas, credenciais, páginas autenticadas e notas fiscais não são armazenados no cache offline; consultas e salvamentos precisam de conexão, e não há fila de envios automáticos.

Os ícones podem ser regenerados com `go run scripts/generate-icons.go`, usando somente a biblioteca padrão do Go. Para validar a instalação em um aparelho, teste em HTTPS confiável a abertura pelo ícone, login, encerramento de sessão, página offline e atualização após trocar o executável. A instalação real no navegador/aparelho requer essa conferência manual.

## Proteção das alterações e autenticação

Cada salvamento de reembolso informa a versão consultada. Se outra sessão alterar o valor, o servidor responde com conflito e mantém o valor já salvo. A tela conserva o rascunho e mostra o valor atual; **Usar valor atual** descarta o rascunho e permite começar uma nova edição. Consultas retornam `reembolso_versao`; chamadas de `PATCH /api/pedidos/{id}/reembolso` devem incluir `versao` e `valor_reembolsado`.

A autenticação por senha permite 30 tentativas por minuto e endereço de conexão; operações de passkeys permitem 60. Até oito dessas operações são processadas simultaneamente. Respostas de excesso usam HTTP 429 e `Retry-After`. Cabeçalhos como `X-Forwarded-For` não alteram a cota. Com proxy reverso, conexões do mesmo endereço do proxy compartilham esses limites; configure também limites por cliente no proxy. Desafios de passkeys expiram em cinco minutos, têm cota por cliente e usam capacidades separadas para login e cadastro.

## Acesso e administração

Na primeira inicialização de um banco sem autenticação, é criado o usuário **admin** com uma senha aleatória, exibida uma única vez no terminal de inicialização. Guarde essa senha e altere-a em **Minha conta**. Opcionalmente, defina `REEMBOLSO_ADMIN_PASSWORD` no ambiente do processo antes da primeira execução, com uma senha de 8 a 128 caracteres; nesse caso ela não é impressa. Usuários e senhas de bancos já inicializados são preservados, inclusive após reiniciar ou atualizar. Cadastros, pedidos e anexos existentes também são preservados.

Depois de entrar, use **Minha conta → Alterar senha** para alterar a senha do usuário conectado. Novas senhas têm de 8 a 128 caracteres. A alteração encerra todas as sessões desse usuário, exigindo novo login.

Em **Administração**, administradores podem:

- Criar usuários normais ou outros administradores.
- Excluir outros usuários, encerrando também suas sessões.
- Excluir prestadores, solicitantes, dependentes e contas para crédito das opções de preenchimento.

Não é possível excluir o próprio usuário nem deixar o sistema sem administrador. Exclusões de cadastros não alteram os pedidos já salvos. Usuários normais podem preencher e imprimir pedidos, anexar notas, adicionar cadastros durante o preenchimento e alterar sua própria senha. Os cadastros e pedidos são compartilhados entre os usuários autenticados.

As permissões são verificadas no servidor, inclusive para chamadas diretas à API. Senhas são armazenadas como hashes PBKDF2-SHA256 com salt aleatório e 600.000 iterações. Sessões têm validade de 12 horas e ficam em cookies HttpOnly/SameSite Strict. O botão **Sair** revoga a sessão no banco.

## Login com biometria e passkeys

Em **Minha conta → Biometria e passkeys**, informe um nome para identificar o aparelho e sua senha atual. Clique em **Cadastrar biometria / passkey** e confirme no dispositivo. Na tela de entrada, use **Entrar com biometria / passkey**; não é necessário preencher o usuário ou a senha nesse fluxo.

O dispositivo ou gerenciador de passkeys escolhe a confirmação disponível: digital, reconhecimento facial ou PIN. A aplicação exige verificação do usuário, mas não controla qual método local será usado. Dados biométricos e a chave privada não são enviados ao servidor. A senha continua funcionando e não é removida ao cadastrar uma passkey.

Passkeys são vinculadas ao nome do site. Use um domínio fixo e HTTPS com certificado confiável. Ao iniciar o servidor, configure explicitamente o endereço público:

```sh
./iniciar.sh -tls-cert /caminho/certificado.pem -tls-key /caminho/chave.pem \
  -webauthn-origin https://reembolso.exemplo.com:5002
```

Com um proxy HTTPS, informe a mesma opção com a origem pública e preserve o cabeçalho Host ao encaminhar as requisições. A aplicação não usa cabeçalhos X-Forwarded para decidir qual origem é confiável. Em desenvolvimento, `http://localhost:8080` funciona sem essa opção. Endereços IP e HTTP na rede local não habilitam o login por passkey. Se mudar o domínio, será necessário cadastrar passkeys para o novo domínio.

A lista em **Minha conta** permite remover passkeys mediante confirmação da senha. A remoção encerra as sessões abertas com aquela passkey, incluindo a sessão atual quando for o caso. Se perder um aparelho, entre com a senha ou outra passkey e remova o acesso correspondente. Alterar a senha encerra todas as sessões e confirmações em andamento; as passkeys cadastradas permanecem válidas até serem removidas.

As confirmações usam desafios aleatórios vinculados à origem e ao navegador, com validade de cinco minutos e uso único. Cadastro e remoção exigem a senha atual. O servidor verifica as respostas com a biblioteca [go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) e persiste a credencial completa e seus contadores. Há limite de 32 passkeys por usuário. As novas tabelas são criadas automaticamente, preservando contas, pedidos e sessões existentes. As passkeys e seus vínculos acompanham o banco nos backups.

Para compilar e verificar a integração em uma máquina com acesso à internet:

```sh
go mod tidy
GOCACHE=/tmp/reembolso-go-cache go test -buildvcs=false ./...
node --test web/*.test.cjs
python3 -m unittest discover -p "*_test.py"
GOCACHE=/tmp/reembolso-go-cache go build -buildvcs=false -o reembolso .
```

Depois, reinicie o servidor e confira cadastro, entrada e remoção em um dispositivo real com biometria ou PIN. A instalação PWA e o uso offline não dispensam conexão com o servidor para autenticar.

## Uso

1. Selecione um solicitante salvo ou preencha os dados.
2. Marque a opção de dependente se o atendimento for dele e selecione ou preencha seu cadastro.
3. Selecione ou preencha prestador e conta de crédito.
4. Informe o valor pago, o local e a data.
5. Opcionalmente, selecione a nota fiscal no campo próprio e os laudos em **Laudos e outros documentos**. São até 3 PDFs no total, com até 10 MB e 20 páginas cada. Clique em **Salvar e gerar impressão**. Os novos cadastros e o pedido são salvos juntos numa transação SQLite.
6. Na página gerada, clique em **Imprimir / Salvar PDF**. O formulário será a primeira página e os anexos começarão na segunda, com a nota fiscal primeiro. Documentos com várias páginas serão incluídos integralmente. O botão só é habilitado quando todas as imagens estão carregadas.

Na impressão, escolha **A4**, **escala 100%**, **margens nenhuma** e desative **cabeçalhos e rodapés**. O formulário inclui seu próprio fundo como imagem, sem depender da opção de imprimir fundos CSS. Na impressão, navegação e avisos não ocupam espaço; as quebras são aplicadas somente entre o formulário e as páginas dos documentos. A assinatura e os campos de uso da Banescaixa ficam para preenchimento posterior. Impressoras sem impressão até a borda podem aplicar redução: prefira salvar em PDF primeiro se necessário.

Pessoas com CPF são identificadas pela combinação de inscrição e CPF, independentemente de serem usadas como solicitante ou dependente. Variações de nome não criam outro cadastro para essa combinação; inscrições diferentes continuam separadas. O sistema rejeita um CPF diferente para a mesma inscrição e nome equivalente, sem alterar o cadastro ou salvar parte do pedido. Ambos os comboboxes exibem a mesma lista. E-mail e telefone informados atualizam o cadastro compartilhado; campos de contato vazios não apagam os dados já salvos. Na atualização, duplicatas com a mesma inscrição e CPF são unificadas automaticamente, preservando o primeiro identificador e os contatos preenchidos. CPFs conflitantes são mantidos para conferência; os pedidos históricos não são reescritos. Prestadores são identificados pelo nome / razão social e CPF / CNPJ. A descrição opcional identifica o médico no formulário web, no seletor de prestadores e na administração; ela é atualizada ao gerar um pedido e não aparece no formulário impresso. Contas com todos os dados iguais são reutilizadas. Valor, local e data pertencem ao pedido, não ao cadastro do prestador. Pedidos já gerados conservam seus dados independentemente dos próximos cadastros e podem ser reabertos em `/imprimir?id=NUMERO`.

## Consulta e acompanhamento dos pedidos

Acesse **Pedidos** no menu (ou `/pedidos`) para listar todos os pedidos já salvos, do mais recente para o mais antigo, exibindo um ano por vez conforme a data informada no formulário. Quando há pedidos de vários anos, a página abre no ano corrente e mostra links no rodapé para consultar os outros anos. Se há pedidos de um único ano, esse ano é exibido. Os filtros são mantidos ao trocar de ano pelos links. O ano também acompanha a URL e os botões Voltar/Avançar do navegador. Os filtros de **Solicitante** e **Dependente** podem ser combinados. A opção **Sem dependente** mostra os atendimentos do próprio solicitante. As opções usam os dados dos pedidos, incluindo pessoas removidas dos cadastros.

Cada pedido ocupa duas linhas na tabela: a primeira reúne número, data, solicitante, inscrição, dependente, valor total e ações; a segunda reúne o input de **Observações**, mais largo, o campo de valor e o botão **Salvar**. Os dois inputs têm a mesma altura. Observações e valor são salvos juntos. Cada pedido mostra o valor total e um campo **Valor reembolsado (R$)** com botão **Salvar**. Aceita valores em reais, inclusive zero; deixe o campo vazio e salve para remover o valor informado. Os totais consideram somente o ano exibido e são atualizados conforme os filtros e os valores salvos. Valores ainda não informados são identificados e contabilizados como zero nas somas. Valores em edição são preservados ao trocar de filtro ou de ano enquanto a página permanece aberta; só entram nos totais após clicar em **Salvar**.

Administradores também veem **Excluir pedido** na mesma célula de ações de **Abrir / imprimir**, sem acrescentar colunas. A exclusão pede confirmação, remove o pedido e todos os seus documentos e atualiza os totais e os links de anos. Os cadastros de pessoas, prestadores e contas são preservados. Usuários normais não podem excluir pedidos. Se o reembolso tiver sido alterado desde a consulta, a exclusão é recusada para permitir revisão. Os identificadores dos pedidos excluídos não são reutilizados. Se os arquivos da nota não puderem ser removidos por falta de permissão ou caminho inválido, a tela informa que o pedido foi excluído e que o anexo precisa de conferência no servidor.

O link **Abrir / imprimir** reabre o formulário original. O valor reembolsado fica armazenado separadamente, em centavos, sem alterar o formulário ou os anexos. A atualização do banco é automática e preserva os pedidos antigos. Todos os usuários autenticados podem consultar e registrar os valores, seguindo o compartilhamento dos pedidos existente no sistema.

A interface carrega os pedidos do ano selecionado usando `GET /api/pedidos?ano=2026`. Os anos disponíveis e as pessoas dos pedidos históricos são consultados em `GET /api/pedidos/filtros`. A consulta `GET /api/pedidos` continua disponível para listar todos os anos. A API também oferece `PATCH /api/pedidos/{id}/reembolso`. A atualização recebe `{"valor_reembolsado":"1234.56"}` (decimal normalizado, até R$ 999.999.999,99), ou `{"valor_reembolsado":""}` para limpar.

## Uso em smartphones e tablets

Em telas de até 768 px, a navegação superior começa recolhida e abre pelo botão **Menu**, com links em coluna. O botão informa seu estado aos leitores de tela; Escape recolhe a navegação e devolve o foco ao botão. No desktop, os links permanecem visíveis.

Formulários e filtros usam uma coluna em telas pequenas. As listas de pedidos e da administração passam a cartões em telas de até 1100 px. Os pedidos usam uma coluna até 850 px e duas acima disso; em celulares de até 450 px, observações ocupa a largura do cartão e o valor fica ao lado de Salvar na linha seguinte. A administração usa uma coluna até 650 px e duas acima disso. Os mesmos campos e botões são usados no desktop e no celular, mantendo os rascunhos ao girar o aparelho.

Campos usam texto de 16 px e os controles principais têm área de toque de pelo menos 44 px. A navegação inclui indicação da página atual, foco visível e link **Pular para o conteúdo** para uso por teclado. Os cartões mantêm os cabeçalhos e as associações das tabelas para tecnologias assistivas. O zoom do navegador permanece disponível.

Em navegadores atuais, a prévia do formulário se ajusta à largura da tela, preservando as proporções do documento. Ao imprimir ou salvar PDF, o documento continua em A4 com sua tipografia original.

Referências: [refluxo do conteúdo (W3C)](https://www.w3.org/WAI/WCAG22/Understanding/reflow.html), [áreas de toque de 44 px (W3C)](https://www.w3.org/WAI/WCAG22/Techniques/css/C44) e [unidades relativas ao contêiner (MDN)](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Values/length).

Para conferência visual, verifique as telas em larguras de 320, 390, 768 e 1280 px; use nomes longos e confirme a edição do reembolso, a troca de ano, o foco por teclado e a geração do PDF. Os testes automatizados verificam o comportamento e a estrutura acessível; não substituem a conferência em aparelhos e leitores de tela reais.

## Máscaras, validações e seleção de cadastros

Os comboboxes mostram nomes em ordem alfabética, considerando acentos e maiúsculas. Solicitantes e dependentes são ordenados pelo nome. Prestadores são ordenados pela descrição, quando preenchida, ou pelo nome / razão social; empates na descrição são ordenados pela razão social. Contas exibem primeiro o nome do titular e são ordenadas por ele.

- CPF e CPF/CNPJ: máscara automática, verificação dos dígitos e rejeição de sequências repetidas. Aceita CNPJ numérico e alfanumérico em maiúsculas, segundo o [manual da Receita Federal](https://www.gov.br/receitafederal/pt-br/centrais-de-conteudo/publicacoes/documentos-tecnicos/cnpj/manual-dv-cnpj.pdf).
- Telefone opcional: máscara com DDD e validação do formato de telefone fixo ou celular brasileiro (10 ou 11 dígitos).
- Inscrição: somente números, até 17 dígitos. Zeros iniciais são preservados; não é calculado dígito verificador do plano.
- E-mail opcional: validação do formato e domínio. Não verifica a existência da caixa postal.
- Valor pago: aceita digitação com vírgula e colagem de valores como `R$ 1.234,56`. Ao sair do campo, formata em reais. Exige valor positivo, até R$ 999.999.999,99 e no máximo duas casas decimais. A API recebe a representação decimal normalizada (ex.: `1234.56`).
- Agência e conta: preservam zeros, aceitam dígito numérico ou X e convertem X para maiúscula. Não é calculado dígito bancário porque a regra depende do banco.
- Data: rejeita datas inexistentes. Nome, razão social, titular, local e banco têm limites de tamanho e rejeitam caracteres de controle; banco também aceita somente seu código numérico.

Erros são exibidos ao lado dos campos e impedem o envio. O servidor repete as validações antes de salvar. Cadastros antigos continuam no banco; ao selecioná-los em um novo pedido, dados inválidos precisam ser corrigidos. Pedidos antigos permanecem preservados.

## Dados e arquitetura

- `main.go`: servidor HTTP, validação, arquivos embutidos e execução da persistência.
- `validation.go`: validação e normalização dos dados antes da gravação.
- `web/validation.js`: máscaras, validações da interface e ordenação dos comboboxes.
- `pwa.go`: versão e entrega do service worker.
- `web/pwa.js`, `web/sw.js` e `web/manifest.webmanifest`: instalação, atualização e fallback offline público.
- `passkeys.go` e `web/passkeys.js`: cadastro, gerenciamento e login WebAuthn/passkeys.
- `auth.go`: autenticação, sessões e permissões administrativas.
- `store.go`: execução da persistência e inicialização de cada banco uma vez por processo.
- `store.py`: acesso SQLite com consultas parametrizadas e transações, usando apenas a biblioteca padrão Python.
- `web/`: formulário, seleção de cadastros e página A4 de impressão.
- `testdata/nota.pdf`: PDF sintético de duas páginas usado nos testes, sem dados pessoais.
- `web/formulario.png`: arte original a 180 dpi, extraída sem anotações e botões do PDF.
- `data/reembolso.sqlite`: banco criado automaticamente no primeiro uso.
- `data/anexos/`: PDFs originais das notas e imagens usadas para impressão. Esta pasta acompanha o banco nos backups. Ao usar `-db`, fica na mesma pasta do banco.

As tabelas de dados são `pessoas` (cadastro único usado para solicitantes e dependentes), `prestadores`, `contas` e `pedidos`; o controle de acesso usa `usuarios`, `sessoes` e `configuracao`. Pessoas e contas mantêm inscrição e números como texto, preservando zeros à esquerda. Os pedidos armazenam uma cópia dos dados preenchidos. As migrações são executadas na inicialização, e a busca anual usa um índice no banco; consultas de leitura não exigem bloqueio de escrita. CPF/CNPJ são verificados matematicamente; não há consulta de situação cadastral, verificação da elegibilidade do reembolso ou integração com a Banescaixa.

O servidor usa somente o endereço local por padrão. Para backup, encerre a aplicação e copie a pasta `data` inteira, incluindo eventuais arquivos auxiliares SQLite. Os dados são pessoais e bancários; guarde o backup em local apropriado.

## Verificação

```sh
GOCACHE=/tmp/reembolso-go-cache go test -buildvcs=false ./...
node web/validation.test.cjs
node web/print.test.cjs
node web/pedidos.test.cjs
node web/sw.test.cjs
node web/pwa.test.cjs
node web/passkeys.test.cjs
python3 -m unittest discover -p "*_test.py"
```

Os testes cobrem máscaras, dígitos verificadores, normalização e limites dos campos, ordenação alfabética, login inicial, migração de banco existente, bloqueio de acesso anônimo, permissões de usuários normais, troca de senha, revogação de sessões, logout, exclusões sem alterar pedidos anteriores, gravação e recuperação, deduplicação dos cadastros, rejeição de dados inválidos sem persistência, disponibilidade dos arquivos da interface, upload de nota com duas páginas, recuperação do PDF original, páginas de impressão e limpeza dos anexos em caso de erro. Os testes de upload usam `testdata/nota.pdf`, sem depender do documento original ou de notas pessoais. Os testes de acompanhamento também cobrem rascunhos, navegação por histórico, consultas por ano com índice, respostas assíncronas fora de ordem e leitura durante gravações no SQLite. A sintaxe dos scripts pode ser verificada com `node --check web/app.js` e `node --check web/print.js`.

A compilação e os testes HTTP internos passaram no ambiente de criação. A abertura de portas TCP foi bloqueada pelo ambiente; a navegação e a impressão no navegador ainda precisam ser conferidas na máquina de execução.

Ao selecionar uma nota no modelo nacional DANFSe com texto, o sistema tenta preencher emitente, tomador e valor. O emitente é procurado pelo CPF/CNPJ e o tomador pelo CPF. A identificação automática de pessoas exige correspondência de CPF; nomes sozinhos não selecionam cadastros. O CPF de solicitantes e dependentes é obrigatório no formulário de reembolso. A Administração apenas exibe os CPFs cadastrados. PDFs digitalizados e outros modelos podem exigir preenchimento manual.

A conta para crédito é preenchida automaticamente com a primeira conta da lista cujo CPF do titular corresponda ao CPF do solicitante, ao selecionar seu cadastro, informar seu CPF ou ler a nota fiscal. É possível trocar a conta ou editar os dados manualmente; essa escolha é preservada durante novas leituras da nota.

A extração de notas usa as coordenadas das palavras (`pdftotext -bbox-layout`) na primeira página: delimita as seções de prestador e tomador, procura rótulos conhecidos e restringe os valores à coluna correspondente. Aceita variações de rótulos, valores após dois-pontos e nomes em linhas próximas. CPF/CNPJ precisam passar pela validação dos dígitos. Valores conflitantes para o mesmo campo ficam em branco; quando disponível, o valor líquido tem preferência sobre o total dos serviços. A extração continua sendo uma sugestão para revisão: não inclui OCR e layouts sem rótulos reconhecidos exigem preenchimento manual.

Ao trocar ou remover o PDF, as sugestões ligadas à nota anterior são limpas, incluindo a conta derivada dela. Campos corrigidos manualmente são preservados. O salvamento aguarda a conclusão da leitura; uma nova seleção cancela a leitura anterior. Valores monetários precisam estar completos e válidos: sinais negativos e separadores incorretos não geram sugestões, e um líquido explicitamente zero não é substituído pelo bruto.

O servidor aceita até dois envios de pedidos/extrações de notas simultâneos; quando estiver ocupado, informa para tentar novamente. A leitura automática limita a saída do Poppler a 4 MB, 12.000 palavras, 2.000 linhas, 64 seções e 256 rótulos reconhecidos, com cancelamento e prazo compartilhado de 15 segundos. PDFs que excedam esses limites continuam permitindo preenchimento manual. Cada PDF tem limite de 10 MB e 20 páginas. O envio e a conversão dos até 3 PDFs compartilham um prazo de 45 segundos, anterior à gravação do pedido.

### Documentos do pedido

É possível selecionar até três PDFs por pedido (nota fiscal e laudos), com até 10 MB e 20 páginas por arquivo. Selecione a nota fiscal no campo próprio e os laudos no campo **Laudos e outros documentos**. Apenas a nota fiscal é usada no preenchimento automático. Também é possível anexar até três laudos sem nota fiscal e preencher os dados manualmente. Todos os anexos são salvos e impressos após o formulário, com a nota fiscal primeiro. Pedidos antigos com uma única nota continuam disponíveis. Ao excluir um pedido, todos os seus anexos são removidos.

## Rede credenciada

Para solicitar à IA a atualização completa de uma cidade, siga [ATUALIZAR_REDE_CREDENCIADA.md](ATUALIZAR_REDE_CREDENCIADA.md). O procedimento inclui nova coleta, pesquisa de contatos, reconciliação com exclusão de ausentes comprovados, backup, validação e recompilação.

O menu **Rede credenciada** está disponível na barra superior para todos os usuários autenticados, em `/rede-credenciada`.

- Cidade: lista todos os prestadores daquela cidade por nome.
- Cidade e especialidade: combina os dois filtros e ordena por nome.
- Especialidade: agrupa por cidade e ordena os prestadores por nome em cada grupo.
- Sem filtros: mostra toda a rede, agrupada por cidade e ordenada por nome.

A especialidade corresponde ao item completo selecionado (por exemplo, CARDIOLOGIA não inclui ECOCARDIOGRAFIA). A ordenação ignora acentos e diferenças entre maiúsculas e minúsculas. Os filtros podem ser compartilhados na URL. O botão **Pesquisar** aplica a seleção e **Limpar filtros** restaura toda a lista.

Os cartões mostram identificação, especialidades, endereço, mapa e telefones originais da Banescaixa. Para Colatina, os contatos adicionais pesquisados ficam em **Contatos encontrados na web**, com fonte, data, confiança e ressalvas. Esses números não substituem os originais. Contatos de baixa confiança são exibidos sem link de discagem. Números de formato não reconhecido são preservados como recebidos e sinalizados para confirmação, sem discagem automática. Telefones fixos com DDD, celulares com DDD e 0800 completos recebem links de discagem; números incompletos ou suspeitos não são corrigidos por suposição. A publicação na web não comprova que a linha funciona nem que o atendimento ao plano permanece disponível.

O catálogo `data/banescaixa.sqlite3` é uma cópia do banco coletado em 26/11/2026: 855 unidades, com verificação web restrita a Colatina. É embutido no executável junto com `rede.py` e consultado em modo de leitura usando o Python 3 já requerido pelo projeto. Quando `sqlite3.Connection.deserialize` está disponível, a leitura ocorre em memória. Em versões anteriores ao Python 3.11 ou sem suporte a essa API, usa uma cópia temporária privada, aberta somente para leitura e removida ao finalizar a consulta. O banco de pedidos e seus cadastros permanecem separados. Não é necessário distribuir outro banco junto aos binários, e o arquivo do catálogo não é servido por HTTP. Não há atualização automática pela internet.

Para trocar a cópia do catálogo após uma nova coleta, faça um backup SQLite consistente e converta somente a cópia para `PRAGMA journal_mode=DELETE` antes de recompilar; isso evita dependência de arquivos WAL na leitura em memória. Preserve as tabelas de telefones e verificações web. Recompile com o comando de build ou com `scripts/release.py`.

Validação específica:

```sh
python3 -m unittest rede_test.py
go test -buildvcs=false -run TestRede ./...
node --test web/rede.test.cjs
```
