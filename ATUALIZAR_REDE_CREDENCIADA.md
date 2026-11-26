# Atualização da rede credenciada por cidade — instruções para a IA

## Como solicitar

Use este arquivo como instrução para o agente, por exemplo:

> Leia `ATUALIZAR_REDE_CREDENCIADA.md` e atualize a base da cidade de **COLATINA/ES**, incluindo a coleta completa na Banescaixa, pesquisa atual de contatos na web, comparação com o catálogo local, inclusão de novos prestadores, atualização dos alterados e exclusão daqueles que não constam mais na coleta completa. Execute as validações e gere os executáveis atualizados.

A cidade indicada pelo usuário define o escopo. Se ele indicar outra cidade, substitua Colatina em todo o procedimento. Se não houver cidade definida, solicite essa informação antes de alterar a base. Se forem solicitadas várias cidades, trate cada uma de forma independente.

Este documento é um procedimento para execução pelo agente; sua existência não agenda atualizações nem implementa um coletor automático. Ao receber o pedido, execute o trabalho até a conclusão. A solicitação de atualização autoriza as inclusões, alterações e exclusões descritas aqui, sem exigir confirmação adicional para exclusões comprovadas pela coleta completa.

## 1. Identifique os arquivos e o escopo

A raiz deste projeto é `/home/rcaliman/Projetos/reembolso`. Se o projeto tiver sido movido, resolva os caminhos a partir da raiz atual.

| Arquivo | Papel |
| --- | --- |
| `data/banescaixa.sqlite3` | Banco oficial do catálogo usado por este projeto; é o destino da atualização. |
| `rede.py` | Leitura do catálogo e filtros da aplicação. |
| `rede.go` | API e incorporação do catálogo no executável via `go:embed`. |
| `web/rede.js` e `web/rede.html` | Exibição de prestadores, contatos e datas. |
| `scripts/release.py` | Geração de `reembolso`, `reembolso-pi`, `reembolso.zip` e `SHA256SUMS`. |
| `data/reembolso.sqlite` ou caminho configurado por `-db` | Dados de pedidos e usuários. Fora do escopo desta atualização. |

Leia as instruções locais aplicáveis, o esquema real do catálogo e o código atual antes de escrever. O catálogo deste projeto é a fonte local de verdade. Não substitua o arquivo inteiro por outro banco de um projeto vizinho: isso pode desfazer atualizações de outras cidades.

Há um coletor de referência em `/home/rcaliman/Projetos/banescaixa-scraper/scraper.py`. Se disponível, examine-o e reutilize sua lógica de coleta ou adapte-a ao projeto. Se estiver ausente, implemente a coleta conforme este documento; não dependa desse caminho para completar a tarefa.

**Limitações dos utilitários antigos:** o coletor de referência faz inserções e atualizações, mas não exclui ausentes; seu banco padrão pertence ao projeto vizinho. O `importar_verificacao.py` daquele projeto usa evidências antigas e é restrito a Colatina. Nenhum deles executa sozinho este procedimento completo. Não reaplique evidências antigas como se fossem uma nova pesquisa. Não execute o coletor sem a restrição de cidade.

## 2. Faça backup e prepare uma área de trabalho

Antes de alterar o catálogo:

1. Gere uma cópia consistente usando `sqlite3.Connection.backup`, em `backups/rede-credenciada/<data-hora-UTC>-<cidade>/catalogo-antes.sqlite3`.
2. Faça a comparação e a atualização em outra cópia de trabalho. Não use o catálogo real como banco temporário do scraper.
3. Registre a cidade/UF solicitada, a data UTC, as contagens e uma captura lógica dos registros originais. Guarde também os dados das outras cidades para comparar antes/depois.
4. Crie `relatorios/rede-credenciada/<data-hora-UTC>-<cidade>/` para o resultado da coleta, evidências web e relatório de diferenças. Não inclua cookies ou credenciais nos arquivos.

Use o mecanismo de backup SQLite, evitando copiar somente o arquivo principal quando houver WAL. Preserve o backup para eventual restauração. Não altere os dados de pedidos, usuários, sessões, contas ou anexos.

## 3. Refaça a coleta completa na Banescaixa

Ponto de partida:

```text
https://banescaixa.com.br/index.php/beneficiarios/rede-credenciada?view=Manual&cidade=COLATINA&start=0
```

Construa os parâmetros com codificação de URL, usando o valor exato do município no formulário do site. Confirme a UF nos endereços e não associe dados de municípios homônimos. A consulta pública funciona sem cookie; não reutilize cookies antigos deste histórico.

Requisitos da coleta:

- Selecione somente a cidade solicitada, sem filtro de especialidade.
- Busque a primeira página e siga todos os links de paginação daquela cidade, incluindo os que aparecem apenas nas páginas seguintes. Aceite somente links do domínio e da consulta de rede esperados. Registre os offsets visitados e evite ciclos.
- Não deduza o fim pelo número de cartões nem incremente offsets pela quantidade de cartões. O site agrupa linhas de especialidades: uma página com poucos cartões pode ter páginas seguintes.
- Extraia nome, nome fantasia, documento/conselho, endereço, cidade, UF, especialidades, áreas de atuação, telefones e mapa.
- Remova apenas os selos visuais antes de ler o nome. Inclua os cartões de clínica própria e rede referenciada. A clínica própria pode não informar documento.
- Reúna especialidades, áreas e telefones da mesma unidade entre todas as páginas. Só compare a unidade depois dessa consolidação.
- Preserve o telefone da fonte, inclusive quando seu formato for suspeito. Não acrescente nono dígito, corrija DDD ou troque números por suposição.
- Use timeout, tentativas limitadas e intervalo entre requisições, por exemplo 0,5 segundo. Respeite falhas do serviço e não considere bloqueio, login, erro HTTP ou HTML inesperado como uma lista vazia.

Salve o resultado consolidado e a evidência de quais páginas foram consultadas antes de aplicar exclusões.

### Condição obrigatória para excluir ausentes

Uma coleta está completa somente quando todas as páginas descobertas foram obtidas e interpretadas com sucesso, o filtro de cidade está confirmado e não houve falha de extração. Uma exceção em um cartão invalida a completude; não ignore o cartão para prosseguir com exclusões.

Se uma cidade antes populada retornar zero prestadores ou desaparecer do formulário, refaça a consulta sem sessão e confirme independentemente no site: formulário, conteúdo de resultados e ausência real de paginação. Um HTML apenas vazio não basta. Sem evidência suficiente de remoção total, preserve o catálogo, relate o impedimento e não aplique exclusões. Queda inesperada de quantidade exige conferir a paginação e a extração; não se baseie apenas no status HTTP 200.

Quando a lista vazia estiver comprovada como resultado legítimo e completo, a atualização pode excluir os prestadores daquela cidade, registrando a evidência. Não transforme uma falha de rede em exclusão.

## 4. Compare os prestadores com o catálogo local

A tabela `prestadores` identifica **unidades de atendimento**, não apenas pessoas ou empresas. Um documento pode ter vários endereços. Não deduplique somente por nome, CNPJ ou telefone.

Preserve o algoritmo de chave existente. No coletor de referência, `norm` aplica Unicode NFKC, maiúsculas e espaços normalizados; a chave é SHA-256 de `json.dumps(identity, ensure_ascii=False).encode()`, com a serialização padrão, sendo:

```python
identity = [
    norm(tipo_documento),
    re.sub(r'\W', '', documento) if documento else norm(nome),
    norm(cidade),
    norm(endereco),
]
```

Confirme esse algoritmo no código e em registros existentes antes de usá-lo. Mudanças de parser ou normalização não devem provocar exclusões e inserções artificiais em massa. Se detectar um problema de interpretação, resolva a correspondência e documente a migração necessária antes de reconciliar.

Compare os campos de conteúdo de cada chave consolidada, tratando listas como conjuntos ordenados e sem duplicatas:

| Caso | Ação |
| --- | --- |
| Chave nova na coleta completa | Inserir a unidade. |
| Mesma chave com algum campo alterado | Atualizar os campos da Banescaixa pelo conteúdo atual. |
| Mesma chave com conteúdo idêntico | Manter o conteúdo e renovar apenas a data de observação. |
| Chave local ausente na coleta completa validada | Excluir a unidade do catálogo ativo da cidade. |

A lista nova de especialidades e telefones substitui a anterior; não mantenha indefinidamente especialidades ou números que saíram do site. A união vale entre páginas da **mesma coleta**, não entre coletas antigas e novas.

Como o endereço participa da chave, uma mudança de endereço normalmente resulta em exclusão da unidade antiga e inclusão de outra. Registre essa possível mudança no relatório. Não transfira contatos web para o novo endereço sem conferir novamente sua associação.

Preserve `criado_em` nas unidades existentes; altere `atualizado_em` apenas quando o conteúdo mudar; renove `visto_em` somente quando a unidade tiver sido observada nesta coleta. Use datas UTC. Recalcule `conteudo_hash` conforme o algoritmo atual, sem incluir timestamps na comparação de conteúdo.

Registre em `coletas` a cidade, conclusão, páginas e unidades da coleta bem-sucedida. Um registro de coleta não pode indicar sucesso se a etapa correspondente falhou.

## 5. Pesquise contatos atuais na web

Pesquise **cada prestador que permanecer ou for incluído na cidade solicitada**, mesmo quando os contatos da Banescaixa não mudaram. Use nome, nome fantasia, CNPJ ou registro profissional, cidade e endereço para reduzir homônimos.

Priorize o site oficial e a página de contato da unidade. Consulte perfis comerciais, instituições e diretórios públicos como fontes complementares. Sempre que possível, abra a página e confirme o contexto; um resultado de busca isolado tem evidência mais limitada. Registre o método verdadeiro (`pagina_consultada` ou `resultado_de_busca`).

As pesquisas devem ser realizadas nesta atualização. Não invente resultados, não declare telefone ativo sem confirmação e não atualize a data de consulta de uma evidência antiga que não foi efetivamente revisitada. Não entre em contato com os prestadores por mensagem ou ligação; o escopo é consultar fontes públicas.

Para cada número aproveitável, grave em `telefones_web`:

- `prestador_chave`, `telefone`, `fonte_url` e `fonte_titulo`;
- `consultado_em` em UTC e `metodo`;
- `confianca`: `alta`, `media` ou `baixa`, conforme a evidência de identidade e unidade;
- `comparacao_banescaixa`: `coincidente` ou `divergente`, comparando com os telefones da coleta nova;
- `observacao`: justificativa da associação, endereço diferente, contato administrativo/compartilhado, formato antigo, DDD suspeito e demais ressalvas.

Um número diferente pode ser adicional; não conclua automaticamente que o número da Banescaixa está errado. Identifique telefones que pertencem a outra filial ou estabelecimento e não os associe como contato confirmado da unidade pesquisada. Não armazene números mascarados ou incompletos como evidência confiável. Se for útil preservar um número suspeito, classifique como baixa confiança e explique o problema.

Mantenha os contatos web separados dos telefones da tabela `prestadores`. Não substitua os números da Banescaixa por números encontrados em outras fontes. Deduplicate por `(prestador_chave, telefone, fonte_url)`.

Ao revisar uma fonte já existente, atualize a evidência se o contato mudou. Se a fonte confirmar que um número foi retirado ou pertence a outra unidade, remova ou corrija essa associação ativa e preserve a evidência anterior no relatório/backup. Se a fonte estiver inacessível ou o número não puder ser revalidado, preserve o registro anterior com sua data verdadeira e uma ressalva de que não foi confirmado nesta atualização; não apresente-o como evidência nova. Não exclua um contato apenas porque não apareceu no buscador.

Preencha `verificacoes_web` para cada unidade pesquisada com a data, a consulta realizada, a situação e uma observação. As situações existentes são:

- `coincidente`: há evidência atual de confiança alta ou média e todos os números aproveitáveis encontrados coincidem com a Banescaixa;
- `divergente`: há pelo menos um número atual aproveitável diferente ou adicional;
- `sem_evidencia_suficiente`: não há evidência atual suficiente ou só há resultados de baixa confiança.

A situação da nova verificação deve refletir a pesquisa atual; não use um contato antigo não revalidado para afirmar coincidência ou divergência atual. Registre consultas sem resultado suficiente e falhas de acesso com transparência.

## 6. Aplique a atualização com integridade

Prepare um relatório de diferenças antes da gravação. Com a coleta completa validada, aplique a reconciliação na cópia de trabalho em uma transação, com `PRAGMA foreign_keys=ON` e parâmetros SQL. Restrinja explicitamente todas as alterações às chaves da cidade solicitada.

Ao excluir um prestador, exclua primeiro os registros dependentes em `telefones_web` e `verificacoes_web` daquela chave: as relações existentes não têm `ON DELETE CASCADE`. Só depois exclua o registro em `prestadores`. O histórico necessário para auditoria fica no backup e no relatório, não como prestador ativo que deixou de existir na rede.

Não execute `DELETE FROM prestadores` sem restrição, não zere a base e não altere registros de outras cidades. Não execute dois processos de atualização do mesmo catálogo simultaneamente. Antes de publicar a cópia, confirme que o catálogo original não mudou desde o backup; se mudou, reconcilie com a versão atual para não perder trabalho alheio.

Conclua a transação e deixe a cópia com `PRAGMA journal_mode=DELETE`, sem depender de arquivos `-wal` ou `-shm`. Feche as conexões e publique a cópia validada no caminho `data/banescaixa.sqlite3` por substituição atômica no mesmo sistema de arquivos. Em caso de falha, mantenha o catálogo anterior utilizável.

## 7. Valide o resultado e atualize a aplicação

Verificações obrigatórias:

1. `PRAGMA integrity_check` retorna `ok` e `PRAGMA foreign_key_check` não retorna linhas.
2. As chaves e o conteúdo da cidade correspondem à coleta completa consolidada; as exclusões estão justificadas e os novos contatos têm fontes.
3. Todos os registros das outras cidades permanecem idênticos, incluindo seus contatos e verificações web.
4. As listas JSON são válidas, sem duplicatas, e a pesquisa por cidade, especialidade e combinação continua ordenada corretamente.
5. Telefones originais e web permanecem separados; números suspeitos continuam sinalizados e sem discagem automática.
6. Reaplicar a mesma coleta preparada não cria duplicatas nem muda datas de alteração de conteúdo idêntico. Não é necessário refazer as requisições de rede para essa verificação.

Execute na raiz do projeto:

```sh
python3 -m unittest discover -p '*_test.py'
node --test web/*.test.cjs
go test -count=1 -buildvcs=false ./...
go vet ./...
```

Alguns testes atuais usam a fotografia inicial de Colatina (41 unidades e números específicos). Se os dados reais mudarem, não altere os resultados esperados apenas para fazer a suíte passar. Separe testes de comportamento com fixtures estáveis da verificação do catálogo atualizado, comprovando no relatório as mudanças legítimas.

Revise as datas e contagens informadas em `web/rede.html` e no README. Uma atualização de apenas uma cidade não torna o catálogo inteiro atualizado naquela data. Evite afirmar que todas as cidades tiveram contatos pesquisados; preserve e explique o escopo e as datas por cidade. Se necessário, adapte a apresentação para usar os metadados reais de `coletas`/`verificacoes_web`.

O banco é embutido no executável: alterar somente o SQLite não atualiza uma aplicação que já foi compilada. Preserve os artefatos anteriores e gere as novas versões:

```sh
python3 scripts/release.py
sha256sum -c SHA256SUMS
```

Se houver falha de testes ou compilação, não declare a atualização pronta para uso. Corrija falhas relacionadas à atualização; informe separadamente impedimentos externos. Não reinicie ou instale em outro servidor sem autorização para esse ambiente. Informe quando for necessário substituir o executável e reiniciar a aplicação em execução.

## 8. Entregue um resumo verificável

Ao terminar, informe:

- Cidade/UF, data e fonte da coleta; quantidade de páginas e unidades.
- Quantidades de inseridos, atualizados, excluídos e inalterados, com a lista de exclusões no relatório.
- Quantidade de unidades pesquisadas na web e resumo de coincidências, divergências e evidências insuficientes.
- Fontes, contatos não revalidados e demais pendências, sem prometer funcionamento das linhas.
- Caminhos do catálogo, backup, relatório e evidências.
- Resultado dos testes, integridade e geração dos binários/pacote.
- Confirmação de que as outras cidades e o banco de pedidos foram preservados.

Não alegue uma atualização completa se alguma página da Banescaixa ficou pendente. Diferencie a conclusão da coleta da cidade das limitações da pesquisa de contatos na web.
