# Formati canonici v1 — C-01

[Indice](README.md) · [English](../en/canonical-formats.md)

**19 novembre 2026 — baseline v1 normativa; WAL/recovery log locale implementati; snapshot generali e integrazione distribuita restano futuri.**

## Confine e compatibilità

C-01 richiede schema byte-per-byte, kind/nullabilità, bootstrap, checksum e fixture di recovery prima di fissare il codice WAL/snapshot M03. Questo documento fissa quel sottoinsieme storage. Fixture configurazione/protocollo ora implementate e verificate in [M04](milestone-m04.md), con [hash configurazione](configuration.md) esatto; OpenAPI resta M08. [Vettori statici](../fixtures/canonical-v1.json) e [verificatore aritmetico](../fixtures/verify_canonical_v1.py) specificano byte, non un WAL funzionante. [Revisione e decisione sui gate](pre-m03-review.md).

Command/Result M01 sono valori Go locali senza rappresentazione serializzata. M02 accetta soltanto comandi KV indicizzati; nessuno dei due package acquisisce qui sessioni, gestione noop o persistenza. Il futuro adattatore di replica dovrà mappare read su get M01, trattare noop senza mutazione KV avanzando il confine apply globale e coordinare atomicamente le sessioni. Non dovrà inviare un opcode noop sconosciuto a M01 o saltarne l'indice globale. Tale integrazione resta futura.

Il formato persistente v1 è fissato per implementazione e fixture; modifiche incompatibili incrementano la versione del formato interessato e richiedono un piano rivisto di migrazione/nuova directory. Versioni/kind/campi sconosciuti bloccano; nessun rolling upgrade, tentativo euristico o downgrade silenzioso. Le versioni degli envelope selezionano gli schemi entry/risultato annidati. La versione del digest ha separazione di dominio indipendente. HTTP/JSON ammette ordine/escape leciti e metadati di trasporto; viene validato e ricostruito da valori tipizzati, mai copiato direttamente nel WAL o nei byte di hashing.

## JSON canonico e domini scalari

Byte canonici ASCII UTF-8 senza BOM, spaziatura, newline o dati finali. Ogni oggetto ordina ricorsivamente i nomi dei campi per byte ASCII crescenti; gli array conservano l'ordine specificato. Tutte le stringhe dello schema sono limitate ai domini ASCII sotto: le stringhe canoniche usano caratteri letterali senza escape, neppure per slash. Booleani true/false. Niente float, esponenti, interi con segno o zero negativo. L'unico null è hard_state.voted_for. Campi opzionali assenti, mai null. Nomi duplicati, campi sconosciuti, tipi errati e profondità oltre 16 sono errori. L'input persistente deve uguagliare la ricodifica canonica dopo validazione tipizzata completa; non basta accettare l'iterazione di una mappa o compattare JSON.

| Tipo | Rappresentazione canonica / limiti |
|---|---|
| uint64 | Stringa JSON: 0 o decimale non nullo senza zeri iniziali, massimo 18446744073709551615. |
| Versioni | Numero JSON 1. Versioni sconosciute rifiutate prima di interpretare il payload. |
| Sequence client | Intero JSON 1..9007199254740991, senza zeri iniziali. |
| Chiave | Pattern ASCII esatto [A-Za-z0-9][A-Za-z0-9._:/-]{0,255}; case sensitive, nessuna normalizzazione. |
| Byte valore / expected | Base64 standard RFC 4648, padding obbligatorio, uguaglianza decode/re-encode; 0..16384 byte decodificati. Stringa vuota significa byte presenti vuoti. |
| ID / hash | cluster_id e client_id: 32 hex minuscoli; config_hash, command_sha256, snapshot_sha256, checksum: 64 hex minuscoli. |
| Membro / generazione | n1, n2, n3; generazione gen-N, N=1..MaxUint64 senza zeri iniziali o separatori di percorso. |
| Contatori | Indice, termine, base, conteggi e logical_bytes usano stringhe uint64. Indice/termine 0 soltanto per metadati del prefisso vuoto/bootstrap. Entry reali con indice e termine positivi. |

Validare lunghezze dichiarate prima di allocare, poi schema, limiti, checksum e invarianti semantiche prima di installare effetti. Overflow fatale, niente wraparound. Limiti KV invariati: 10000 chiavi e 4194304 byte logici; massimo 1024 sessioni. Nessun percorso filesystem deriva da una chiave KV. ID/hash sintetici delle fixture identificano esempi di formato, non una configurazione M04 verificata.

## Entry e comandi

Entry contiene esattamente index, term, kind e, tranne noop, command. kind è noop, read, put, delete o cas. command contiene esattamente:

| kind | Campi command |
|---|---|
| noop | command assente, non null; nessuna identità client. |
| read | Soltanto key; nessuna sessione. |
| put | key, value_b64, client_id, sequence. |
| delete | key, client_id, sequence. |
| cas | key, expected, new_value_b64, client_id, sequence. |

expected è esattamente {exists:false} oppure {exists:true,value_b64:string}. Nessun campo valore con exists=false. Un valore nil locale put/CAS M01 si proietta in byte vuoti; nil non diventa mai JSON null. Comandi esterni malformati falliscono prima dell'ammissione al log; una entry persistita malformata è errore d'integrità. I test locali INVALID_ARGUMENT M01 restano validi e non implicano che quei comandi vadano persistiti.

Entry canonica massima: 49152 byte inclusi metadati. Un CAS massimo con indice/termine uint64-max, chiave di 256 byte, sequence massima e due valori di 16384 byte occupa **44184 byte**, come calcolato dal verificatore delle fixture. È distinto dal cap corpo HTTP 48 KiB, dal cap grezzo di copia M02 48 KiB e dai cap payload peer/WAL 384 KiB.

## Digest comandi e risultati operativi

Per l'identità del comando usare SHA-256 su questa sequenza tipizzata senza JSON intermedio: ASCII QCMD, un byte 01 (versione), un byte opcode (noop=00, put=01, delete=02, cas=03, read=04). Tranne noop, aggiungere key come lunghezza in byte uint32 big endian seguita dai byte ASCII. put aggiunge il valore grezzo con prefisso di lunghezza. cas aggiunge un byte expected-exists 00/01, expected grezzo con prefisso solo se presente, poi nuovo valore grezzo con prefisso. delete/read non hanno altri byte. Noop termina dopo opcode. Nessun terminatore o padding implicito. I prefissi contano sempre byte.

Il digest esclude index, term, client_id, sequence, timeout e correlazione: la coppia client_id/sequence è verificata separatamente, il digest confronta il payload operativo. Expected assente e presente vuoto hanno preimmagini diverse. Le preimmagini get/read e noop sono definite per confronti deterministici, non per registrare sessioni. SHA-256 è integrità/identità nel modello non bizantino, non autenticazione.

Il risultato applicativo canonico contiene code e index, più la seguente union esatta. Nessun request_id/outcome/stato HTTP persistito. Errori interni worker/context non sono risultati di dominio e non vanno serializzati come applicazione riuscita.

| Risultato | Campi aggiuntivi |
|---|---|
| OK per put/cas | applied:true. |
| OK per delete | deleted:boolean. |
| OK per read | exists:true, value_b64:string. |
| KEY_NOT_FOUND per read | exists:false. |
| COMPARE_FAILED per cas; RESOURCE_LIMIT per put/cas | applied:false. |
| SESSION_LIMIT, REQUEST_CONFLICT, STALE_SEQUENCE, SEQUENCE_GAP | Nessun campo aggiuntivo; index è l'entry valutatrice. Non sostituiscono mai un risultato valido in cache. |
| Completamento noop | Soltanto code OK e index; nessuna sessione in cache. |

Soltanto risultati mutanti OK, COMPARE_FAILED e RESOURCE_LIMIT entrano nella cache sessioni; risultato canonico <=512 byte, indice applicato originale conservato nei retry. INVALID_ARGUMENT/INVALID_SEQUENCE di input ed errori di trasporto non sono in cache. Nessun vecchio valore nei risultati mutanti. M11 gestirà la transizione di sessione; queste sono decisioni di schema, non comportamento implementato.

## Frame transazionali WAL

Esattamente 24 byte di header: offset 0..3 ASCII QWAL, 4..5 uint16 versione=1, 6..7 uint16 kind=1 (transazione), 8..11 uint32 lunghezza payload, 12..19 uint64 sequence fisica, 20..23 uint32 CRC32C, tutti gli interi big endian. Payload JSON canonico, 1..393216 byte. Dimensione frame 24+payload; nessun allineamento, separatore o trailer. Sequence da 1 per generazione, contigua, mai riusata per append nella medesima generazione. Overflow ferma il writer. Ogni generazione inizia con una transazione contenente esattamente hard_state e commit_index uguale alla base snapshot S, senza sostituzione suffisso; frame successivi costruiscono il suffisso prima del checkpoint di commit superiore.

CRC32C usa Castagnoli, polinomio riflesso 0x82f63b78, init/xorout 0xffffffff; valore di controllo per ASCII 123456789: e3069283. Copre byte header 0..19 seguiti immediatamente dal payload; campo checksum escluso. Non è CRC32 IEEE né autenticazione.

La transazione è un oggetto non vuoto con qualunque dei seguenti campi opzionali, tutti i valori presenti non-null:

- hard_state: esattamente current_term e voted_for. voted_for è null o un membro statico. Il termine non regredisce; nello stesso termine un membro registrato non può cambiare o essere cancellato. Un termine superiore può resettare il voto. Termine 0 richiede null.
- replace_suffix: esattamente from_index ed array entries, eventualmente vuoto. Richiedere commitIndex < from_index <= lastLogIndex+1 prima della sostituzione, entry contigue da from_index, termini non decrescenti attraverso il confine conservato e termini entry <= current_term dopo la transazione. Conservare il prefisso, sostituire tutto il suffisso. Mai sovrascrivere/eliminare un indice committed.
- commit_index: non decrescente, tra confine snapshot e lastLogIndex dopo sostituzione. È checkpoint di commit legale stabilito esternamente, non deduce consenso dalla lunghezza del log.

Validare atomicamente il post-stato nell'ordine: hard state, suffisso, commit. Nessun effetto visibile da frame parzialmente valido. Massimo 16 entry e 262144 byte canonici complessivi di entry per transazione, rispettando anche il cap payload. Questi limiti conservativi rientrano nei cap batch peer. I conflitti di log aggiungono una transazione di sostituzione, senza troncamento fisico di storia valida. Una generazione fisica può contenere molte transazioni.

## Immagini, manifest e bootstrap

File snapshot: ASCII QSN1 (4), lunghezza payload uint64 big endian (8), payload canonico, poi 32 byte SHA-256 grezzi su header di 12 byte più payload precedenti. Esattamente 44+payload byte, massimo 8388608. SHA-256 dell'intero file, incluso trailer, è ID snapshot RPC e CURRENT.snapshot_sha256; differisce dal digest nel trailer.

Payload con esattamente format_version, cluster_id, config_hash, last_included_index, last_included_term, logical_bytes, key_count, session_count, kv e sessions. kv è array di oggetti {key,value_b64}, in ordine ASCII di key strettamente crescente, senza duplicati. sessions è strettamente crescente per client_id, senza duplicati; ogni record ha esattamente client_id, sequence, command_sha256, kind e result. kind è put/delete/cas; result è la union in cache sopra, con 1<=index<=indice snapshot. Validare conteggi, valori decodificati, somma logica esatta e tutte le quote. Lo snapshot vuoto ha confine (0,0), array vuoti e contatori zero. Indice incluso positivo richiede termine incluso positivo; termine incluso <= termine corrente recuperato. Read/noop non aggiungono dati utente. Ordinamento array esplicito, mai iterazione filesystem/mappe. Limite conservativo della codifica: ceil(4*4194304/3) + 10000*40 + 1024*768 + 4096 + 44 = 6782978 byte, sotto 8 MiB: 40 copre ogni riga KV/separatore e arrotondamento Base64, 768 un record sessione con risultato <=512 byte, 4096 i metadati principali. È aritmetica di formato, non snapshot misurato o allocazione storage.

CURRENT e IDENTITY sono file JSON canonici senza newline, ciascuno <=4096 byte. Campi dati CURRENT: format_version, generation, snapshot_sha256, base_index, base_term. Campi dati IDENTITY: format_version, cluster_id, node_id, config_hash. Per entrambi checksum è SHA-256 minuscolo dell'oggetto canonico senza checksum; aggiungere poi checksum e codificare canonicamente l'oggetto finale. Rifiutare campi extra, checksum errato, traversal e disaccordi snapshot/base/identità/config. CURRENT è l'unica autorità di selezione; mai scegliere la directory maggiore come fallback di recovery.

La directory generazione contiene snapshot e wal come file dati obbligatori. LOCK è un file stabile di lock advisory nella radice, non un record database serializzato. I nomi generazione sono ID di pubblicazione locali, non indici Raft. Allocare sotto lock un N nuovo non usato sopra gli ID generazione riconosciuti esistenti, incluso staging; mai riusare nomi o fare wraparound. Questa scansione non autorizza a scegliere per recovery una generazione non referenziata.

Bootstrap esplicito richiede directory nodo precedentemente vuota, prima di creare LOCK. Creare IDENTITY con checksum, snapshot vuoto e prima transazione WAL {hard_state:{current_term:"0",voted_for:null},commit_index:"0"}, sequence fisica 1. Sincronizzare e pubblicare con lo stesso protocollo generazionale successivo. Nessun WAL vuoto valido. Un bootstrap parziale, incluso solo LOCK, è rifiutato dall'avvio ordinario; niente cancellazione/rebootstrap/resume automatico.

Alla rotazione scrivere prima transazione con hard state corrente e commit=S, aggiungere tutto il suffisso conservato S+1..last in transazioni limitate, poi checkpoint del commit corrente catturato dopo tutte le entry necessarie. Sync di tutto prima di pubblicare CURRENT. Se S=MaxUint64 il suffisso è vuoto; mai calcolare S+1 con wraparound. Conservare il voto corrente alla barriera finale, non alla cattura snapshot. M13 implementerà rotazione/installazione; M03 deve riservare questi formati senza anticipare gli snapshot.

## Fixture di recovery e criteri di chiusura

Il [file fixture](../fixtures/canonical-v1.json) fissa sei vettori entry/digest, sei forme di risultato, due frame WAL, byte snapshot vuoto e con stato, checksum IDENTITY/CURRENT, sei casi JSON negativi e 18 scenari recovery. Frame completi sopravvissuti senza Sync possono essere riprodotti; la presenza non dimostra l'invio di una risposta. Soltanto l'ultimo frame strutturalmente incompleto può essere troncato all'offset verificato precedente e sincronizzato, nel modello crash processo/non corruzione. WAL vuoto, checksum errato completo, versione/kind ignoti, schema invalido, buchi di sequence, regressione voto/termine, cancellazione suffisso committed o commit oltre log bloccano. Mai cercare un altro magic. Prima transazione incompleta non produce una generazione valida.

Una lunghezza corrotta può sembrare coda incompleta: riparazione automatica di corruzione arbitraria esclusa. Sospetta corruzione o errore dopo un confine acknowledged è fatale; conservare evidenze. CURRENT mancante con altri file, bootstrap incompleto o generazione pubblicata corrotta sono fatali. Staging non pubblicato non supera mai CURRENT; cleanup soltanto dopo validazione della generazione selezionata. Una generazione pubblicata sopravvissuta è autorevole anche se resta quella precedente.

Il progetto storage C-01 è RESOLVED quando: tutte le regole byte/tipi/union/versioni sopra sono fissate; decisioni bootstrap/pubblicazione/recovery sono univoche; byte canonici, lunghezze e checksum dei vettori statici sono verificati indipendentemente; i futuri test reader M03/M13 devono consumare casi positivi e negativi. Il verificatore valida aritmetica delle fixture, non codice di recovery. Conformità effettiva codec/recovery resta T-013/T-015/T-016 alle milestone pertinenti; fixture complete config/protocollo/OpenAPI mantengono i gate successivi originali.

## Sottoinsieme preliminare implementato

[Relazione di fase](milestone-m03-preliminary.md): internal/storageformat implementa ora encode/decode esatto frame QWAL, validazione entry/digest tipizzato e seal/validazione canonici CURRENT/IDENTITY. Byte fixture approvati invariati. DecodeFrame è senza stato: non autorizza troncamento, non valida commit/voti fra frame né seleziona generazioni. Codec snapshot/risultati/sessioni e conformità recovery completa restano futuri. Progetto C-01 risolto; T-013 M03 completo non chiuso.

## Conformità M03 ora implementata

[Evidenze M03 finali](milestone-m03.md): esercitati tutti i 13 casi recovery di immagini byte e le cinque aspettative namespace. Transazioni tipizzate decodificate riusano il validatore canonico esatto; nessuno schema o byte fixture approvato cambiato. EmptySnapshot supporta solo immagine bootstrap (0,0) obbligatoria. FrameSize consente allocazione streaming limitata e rifiuta magic/versione/kind/lunghezza parziali contraddetti prima di trattare coda finale come incompleta. WAL valida atomicamente prima transazione, continuità fisica, voto/termine persistenti, suffisso e commit; solo incompletezza EOF finale valida può essere troncata e sincronizzata. Applicazione snapshot/risultati/sessioni generale resta ai gate successivi.
