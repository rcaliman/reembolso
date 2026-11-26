Correções implementadas após a revisão:

- Sugestões vinculadas à nota são invalidadas ao trocar/remover o PDF; edições manuais são preservadas. A leitura anterior é cancelada e o salvamento fica bloqueado enquanto houver uma extração pendente.
- Valores são validados por inteiro, com rejeição de sinais negativos e formatos incorretos. Um líquido explicitamente zero ou inválido não dá lugar ao bruto.
- Pessoas passam a ser reutilizadas por inscrição e CPF. A migração unifica duplicatas inequívocas, preserva contatos e histórico e mantém inscrições diferentes separadas.
- Conflitos de CPF retornam HTTP 409 e desfazem toda a transação. Anexos de pedidos recusados são removidos.
- O processamento de PDFs tem cota de concorrência, saída limitada e limites de complexidade com cancelamento durante o parser.
- Os testes de upload usam um PDF sintético incluído em testdata. Foram adicionadas regressões para os cenários relatados.

Verificação após as correções: a suíte Go completa e `go vet ./...` passaram; os oito arquivos de testes JavaScript e os 13 testes Python passaram. Os executáveis AMD64/ARM64 e o ZIP do Raspberry foram recompilados. Os testes da interface usam DOM simulado, sem substituir uma conferência em navegador real.

O texto abaixo registra a revisão original e as reproduções anteriores às correções.

Revisão do sistema de reembolso — 19/10/2026

Escopo: servidor Go, autenticação e passkeys, permissões, validação, SQLite e migrações, anexos, extração de notas, formulário, conta automática, acompanhamento de pedidos, impressão, PWA e geração dos pacotes. A revisão não alterou código da aplicação, executáveis nem o banco de produção. As reproduções usaram bancos temporários, documentos sintéticos e simulação da interface; não houve navegação em um navegador real ou acesso ao Raspberry.

1. **[P1 — alta] Trocar a nota pode manter dados automáticos do PDF anterior.**

   Referências: `web/app.js:150`, `web/app.js:180`, `web/app.js:205`, `web/app.js:213`.

   Reproduzido executando o script real da interface com um DOM simulado: após carregar A.pdf, o formulário ficou com Clínica A, CPF do tomador e R$ 450,00. Ao selecionar B.pdf, cuja extração retornou campos vazios, todos esses dados continuaram preenchidos. O aviso pediu preenchimento manual, mas os campos pareciam completos e o arquivo selecionado já era B.pdf. O mesmo ocorre com campos ausentes em uma extração parcial. Além disso, o envio não aguarda uma extração em andamento: pode enviar o arquivo novo junto dos dados antigos.

   Impacto: salvar e imprimir um pedido com pessoa, prestador ou valor de outra nota.

   Correção sugerida: registrar a origem de cada sugestão, invalidar as sugestões do PDF anterior ao trocar o arquivo e impedir o envio enquanto a leitura está pendente. Preservar separadamente os campos editados manualmente e deixar explícito o estado de revisão quando uma leitura falhar.

2. **[P1 — alta] A extração monetária aceita trechos de valores inválidos e ignora o sinal.**

   Referências: `invoice_layout.go:207`, `invoice_layout.go:258`.

   Reproduções com o parser real: `Valor líquido: -100,00` retornou `100.00`; `Valor líquido: 1.23,45` retornou `23.45`. A expressão regular encontra um fragmento numérico e a validação só recebe esse fragmento, já sem o sinal ou a parte inválida. Também reproduzi `Valor líquido: 0,00` junto de `Valor dos serviços: 450,00`: o parser descartou o líquido zero e sugeriu `450.00`.

   Impacto: preenchimento de um valor pago diferente do documento, inclusive transformando valor negativo em positivo.

   Correção sugerida: validar o valor monetário completo, incluindo sinal e separadores; rejeitar valores negativos ou malformados para preenchimento automático. Distinguir campo ausente de campo líquido explicitamente zero, impedindo a substituição silenciosa pelo bruto.

3. **[P2 — média] A persistência pode duplicar pessoas que a leitura da nota identifica pelo mesmo CPF.**

   Referências: `store.py:270`, `web/app.js:195`.

   Reproduzido em SQLite temporário: salvar Maria da Silva e depois MARIA DA SILVA, com a mesma inscrição e o mesmo CPF, gerou duas pessoas. A unicidade usa a combinação exata de inscrição e nome; a busca da nota usa CPF. Consequentemente, o próximo upload encontra múltiplas pessoas e deixa de selecionar o cadastro automaticamente.

   Impacto: cadastros duplicados, contatos divergentes e perda do reconhecimento automático mesmo com CPF correto.

   Correção sugerida: alinhar a regra de identificação e deduplicação com o CPF. Se várias inscrições para um mesmo CPF forem permitidas, tratar isso explicitamente; uma alteração de caixa ou grafia do nome na mesma inscrição não deve criar outra pessoa.

4. **[P2 — média] Salvar um pedido pode substituir silenciosamente o CPF de uma pessoa existente.**

   Referência: `store.py:272`.

   Reproduzido em SQLite temporário: salvar um segundo pedido com a mesma inscrição e nome, mas outro CPF matematicamente válido, substituiu o CPF compartilhado no cadastro existente. Não há verificação de conflito de identidade; os pedidos históricos conservam o CPF anterior.

   Impacto: um erro de preenchimento altera a associação dos próximos uploads e das contas bancárias, sem criar um alerta de inconsistência entre o cadastro e os pedidos anteriores.

   Correção sugerida: rejeitar o conflito de CPF com mensagem específica ou estabelecer um fluxo explícito de correção. Não tratar CPF diferente como uma atualização comum de contato.

5. **[P2 — média] O processamento de PDFs não tem limites suficientes de memória, trabalho e concorrência.**

   Referências: `invoice_extract.go:48`, `invoice_extract.go:53`, `invoice_layout.go:118`, `invoice_layout.go:163`, `upload.go:92`, `rate_limit.go:61`.

   O limite de 10 MB restringe o arquivo enviado, mas não a quantidade de texto descomprimido produzida por `pdftotext`. `Output()` acumula tudo em memória. O prazo de 15 segundos interrompe o subprocesso, mas não é consultado pelo parser Go. O parser percorre regiões/linhas em laços aninhados; reproduções sintéticas confirmaram crescimento acima de linear. As rotas de extração e conversão de PDFs não participam do limite de oito operações concorrentes aplicado à autenticação.

   Impacto: PDFs densos ou vários uploads simultâneos podem consumir memória e CPU do Raspberry e prejudicar as demais requisições. Não fiz teste de exaustão no servidor real.

   Correção sugerida: limitar o tamanho da saída do Poppler e a quantidade de palavras/linhas aceitas; verificar cancelamento durante o parser; aplicar uma cota pequena de processamento simultâneo de PDFs e limitar o trabalho das buscas geométricas.

6. **[P2 — média] A suíte completa de testes Go não é reproduzível com os arquivos presentes.**

   Referências: `upload_test.go:36`, `upload_test.go:82`.

   `go test ./...` falhou em `TestInvoicePersistsAndPrintPages` e `TestInvoiceInvalidAndCleanup`: ambos dependem de `form_pedido_reembolso.pdf`, que não está na pasta. A existência de outro PDF de nota não resolve a dependência desses testes.

   Impacto: a verificação completa falha antes de cobrir os cenários de persistência/impressão e limpeza do upload, dificultando separar regressões novas de uma falha preexistente de infraestrutura de teste.

   Correção sugerida: incluir uma fixture sintética pequena em `testdata` ou gerá-la durante os testes, sem depender do documento original ou de notas com dados pessoais.

Verificação realizada:

- `GOCACHE=/tmp/reembolso-go-cache go vet ./...`: passou.
- `GOCACHE=/tmp/reembolso-go-cache go test ./...`: falhou nos dois testes de upload descritos acima; os demais testes executados passaram.
- `node --test web/*.test.cjs`: sete arquivos de teste passaram.
- `python3 -m unittest store_test.py passkey_store_test.py`: 13 testes passaram; houve avisos de conexões SQLite não fechadas no próprio teste de passkeys.
- Reproduções adicionais: troca de PDF na interface simulada; valores negativos, malformados e líquido zero no parser; duplicação de pessoas e troca de CPF em banco isolado; custo de leitura de layouts sintéticos.

A autenticação, o compartilhamento entre usuários e as permissões foram avaliados segundo o comportamento documentado: cadastros e pedidos são compartilhados e usuários normais podem registrar reembolsos. Não encontrei um desvio confirmado das permissões administrativas nas rotas revisadas. Isso não equivale a uma auditoria de segurança completa nem a uma validação de vulnerabilidades nas dependências.
