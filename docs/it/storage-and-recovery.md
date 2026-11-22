# Persistenza, snapshot e crash recovery

[Indice](README.md) · [English](../en/storage-and-recovery.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Contratto e assunzioni

Motore proprietario minimale: mappa in RAM, WAL append-only e snapshot su filesystem locale; nessun database esterno. Obiettivo principale: crash del processo, con write/Sync/rename completati rispettati dal sistema operativo. Non equivalere `kill -9` a power loss. La garanzia hardware contro perdita improvvisa di alimentazione dipende da APFS, macOS, controller e cache SSD e resta **Proposed**, senza prove distruttive sul computer personale.

[Go File.Sync](https://pkg.go.dev/os#File.Sync) espone sincronizzazione, ma non costituisce una prova end-to-end di resistenza a guasto fisico. Ordine delle operazioni filesystem conta: vedere [Pillai et al., OSDI 2014](https://www.usenix.org/conference/osdi14/technical-sessions/presentation/pillai). Verificare supporto e comportamento di sincronizzazione directory su darwin/arm64 in M03; non ignorare errori. Eventuale necessità di primitive Darwin aggiuntive richiede revisione ADR-005 e inventario dipendenze.

## Directory, identità e formato WAL proposto

Directory privata 0700, file 0600, un proprietario attivo tramite lock esclusivo del processo (futuro adattatore macOS; test su crash libera il lock). Manifest immutabile di identità contiene versione, cluster_id, node_id e config_hash. Non copiare una directory n1 per avviare n2. File futuri: `IDENTITY`, `CURRENT`, generazioni `gen-N/` con snapshot e WAL; questi file non sono creati in questa fase.

Frame WAL v1: header 24 byte big-endian: magic `QWAL` (4), version uint16 (2), kind uint16 (2), payload length uint32 (4), frame sequence uint64 (8), CRC32C (4). Checksum copre i primi 20 byte header e payload; non è autenticazione. Kind 1 = transaction; payload JSON UTF-8 canonico, massimo 384 KiB, indici/termini come stringhe decimali. sequence fisica è contigua da 1 per generazione.

Transaction contiene campi opzionali `hard_state:{current_term,voted_for}`, `replace_suffix:{from_index,entries}`, `commit_index`. Prima validare integralmente il frame; poi installarne tutti gli effetti in recovery, mai metà transazione. Sostituzione valida solo oltre commit corrente, entries contigue; può sostituire il suffisso con vuoto. Il WAL fisico conserva vecchi record invalidati logicamente: nessun truncate in-place per conflitti Raft. Compaction ricrea un WAL con il solo suffisso vivo.

Versione sconosciuta, indice incoerente, hard state regressivo, checksum errato o commit oltre log sono errori fatali. Un'incompletezza strutturale **soltanto dell'ultimo frame** può essere troncata all'ultimo confine valido e sincronizzata, nel modello crash di processo che esclude corruzione arbitraria. Un frame completo con CRC errato non si “ripara” scartandolo. Un header corrotto potrebbe sembrare una scrittura incompleta: per questo il modello non garantisce recupero automatico da bit rot e richiede fail-closed se si sospetta corruzione. Nessun salto di record per cercare il magic successivo.

## Ordine persistente obbligatorio

| Evento | Prima di rispondere o pubblicare |
|---|---|
| Incremento termine / voto | Hard state scritto e Sync riuscita. |
| AppendEntries success | Termine/voto e modifica log pertinenti in WAL sincronizzato. |
| Replica locale contata dal leader | Frame contenente entry locale già sincronizzato. |
| Commit/apply | Checkpoint commitIndex sincronizzato dopo entry necessarie; solo commit legalmente appreso. |
| Risposta completed client | Entry su maggioranza durevole, commit legale e apply completato; dedup ricostruibile. |
| Snapshot installato | Immagine verificata e generazione pubblicata durevolmente; mai acknowledgment dallo staging. |

`commitIndex` persistito è un checkpoint monotono prudenziale, non un modo per dedurre commit da log.length. Hard state e log possono condividere un frame. Il checkpoint non è replicato come comando e non sostituisce le regole Raft. Nessun successo su write short, ENOSPC, Sync fallita o close rilevante fallita. Storage fatal: smettere di votare, proporre e confermare; diagnosi e riavvio esplicito dopo risoluzione.

## Recovery

1. Acquisire lock e validare identità/configurazione; nessun listener client prima della verifica.
2. Leggere CURRENT e validare generazione, snapshot e WAL. CURRENT mancante su directory non vuota è errore, non autorizza un nuovo cluster. Nessun fallback automatico a snapshot vecchio se la generazione corrente è corrotta.
3. Caricare snapshot: mappa, contatori quota, tabella sessioni con risultato e digest, lastIncludedIndex/Term, cluster/config e versione. Verificare SHA-256, conteggi e limiti.
4. Riprodurre frame validi per ricostruire hard state e suffisso log, mantenendo commit massimo legalmente checkpointed. Reinizializzare lastApplied all'indice snapshot e applicare fino a commit persistito. Non applicare il resto.
5. Avviare follower con timer nuovo; il leader comunicherà commit ulteriore. Un cluster riavviato può ri-committare un prefisso durevole con noop corrente. Rispondere a GET soltanto dopo la nuova barriera, mai dal solo replay.

Se si perde permanentemente la maggioranza dei dati durevoli, nessuna garanzia di conservazione o ripristino automatico. Una singola directory corrotta si mette in quarantena; procedura manuale di ripristino da snapshot verificato di peer sano, solo dopo aver preservato evidenze e identità corretta. Non cambiare membership o creare da soli un quorum con nodi vuoti.

## Snapshot e pubblicazione atomica

Scatto a indice S=lastApplied, certamente committed. Cattura sotto barriera applier di mappa+sessioni+S+term[S]+config, poi copia immutabile. Formato immagine v1: header `QSN1`, lunghezza uint64 big-endian, payload JSON canonico (chiavi ordinate, valori Base64), SHA-256 di header+payload in trailer di 32 byte. Massimo file 8 MiB. Noop e read non lasciano dati utente nello snapshot.

Procedura generazione: (a) riservare spazio; (b) creare gen-N.tmp, scrivere snapshot e WAL contenente suffisso S+1..last, hard state e checkpoint commit; (c) sincronizzare file e directory temporanea; (d) rename a gen-N e sync directory padre; (e) scrivere CURRENT.tmp con formato `{format_version:1,generation:"gen-N",snapshot_sha256,base_index,base_term}` e checksum SHA-256 della codifica canonica dei campi precedenti, Sync, rename su CURRENT e sync padre; (f) soltanto ora eliminare generazione precedente e sync padre. I file di una generazione pubblicata sono immutabili salvo append al suo WAL; durante la breve rotazione finale il worker storage serializza append e copia del suffisso, senza perdere entry arrivate durante la serializzazione snapshot. Fallimento prima della pubblicazione lascia vecchio CURRENT; temporanei non referenziati sono eliminabili dopo verifica.

InstallSnapshot usa lo stesso meccanismo. Se `(S,termS)` coincide con il log locale, conserva il suffisso successivo; altrimenti lo elimina solo se non distrugge commit locale. Rifiutare snapshot obsoleto `S <= lastApplied`; non regredire commit. Se S supera commit locale, lo snapshot valido dal leader stabilisce prefisso committed e nuovo commit almeno S. Coordinare applier e core prima di sostituire stato; nessun apply parallelo di vecchio suffisso. Persistenza termine/voto deve sopravvivere al cambio generazione.

## Compaction e spazio

Trigger a 6 MiB WAL fisico o 2 000 entry dopo ultimo snapshot, solo se lastApplied avanza; una compaction attiva, intervallo minimo 30 s. Hard cap WAL 8 MiB inclusa riserva 512 KiB per metadati/consenso; se spazio dati esaurito, rifiutare nuove proposte, continuare replica/commit entro riserva. Se non c'è prefisso compattabile e la riserva finisce, storage fail-closed: non cancellare entry per creare spazio.

Cap aggregato per directory nodo 32 MiB **inclusi** staging e vecchie generazioni; riservare picco prima di iniziare. Snapshot install e compaction non sono concorrenti. I cap logici replicati sono distinti dai cap fisici locali: dataset pieno produce RESOURCE_LIMIT deterministico; disco pieno rende il singolo nodo indisponibile. Tabella completa e obiettivo cluster circa 100 MB in [risorse](performance-and-resources.md).

## Cutpoint richiesti

Crash prima/dopo write header, payload, Sync, checkpoint commit, rename generazione, rename CURRENT, sync directory e cleanup; includere crash dopo successo client e retry dopo recovery. Per snapshot provare anche trasferimento incompleto, checksum errato, chunk duplicato e leadership cambiata. I test devono ispezionare file recuperati e storia dei successi; l'assenza di panic non basta.

## Bootstrap e validazione del formato

Su directory realmente vuota e bootstrap esplicito: creare IDENTITY durevole, generazione iniziale con snapshot vuoto a `(0,0)`, hard state `(term=0,votedFor=empty)`, commit 0 e WAL sequence iniziale; pubblicare CURRENT con la stessa procedura delle generazioni successive. Un crash a metà bootstrap lascia directory incompleta: avvio ordinario rifiuta, e un comando di ripresa esplicito futuro potrà completarla soltanto dopo verifica dei file, mai cancellarla silenziosamente.

CURRENT è limitato a 4 KiB e non contiene un checksum autoreferenziale: calcolare digest sui soli campi dati, poi aggiungere il campo checksum. I campi base_index/base_term corrispondono all'immagine; snapshot_sha256 è digest dell'intero file immagine. Numeri indice/termine restano stringhe. La [specifica canonica](canonical-formats.md) fissa ora fixture byte-per-byte per header, JSON, digest e aspettative recovery; M03 deve verificarvi il proprio reader. La serializzazione non può usare output non ordinato di mappe.

## Barriera di cambio generazione e installazione

La serializzazione della copia snapshot a S può avvenire mentre il core progredisce; la **rotazione finale** è una transizione serializzata. Prima di copiare il suffisso definitivo: sospendere proposte, transizioni persistenti e compaction concorrenti; drenare le persistenze già inviate alla vecchia generazione e acquisire dal core `(currentTerm,votedFor,commitIndex,lastLogIndex)` aggiornati. Non copiare hard state catturato all'inizio dello snapshot.

Con questa barriera mantenuta, scrivere nella nuova generazione il suffisso vivo S+1..lastLogIndex, hard state e checkpoint commit correnti, poi sincronizzare e pubblicare CURRENT come già specificato. Nessun voto o append riceve ack durante il passaggio. Soltanto dopo pubblicazione e switch del writer rilasciare la barriera. Ogni richiesta/completamento storage usa `(generation_id,write_sequence)`; la sola sequenza, che riparte da 1, non identifica un effetto globalmente. Se compare un completamento vecchio dopo lo switch, non può autorizzare messaggi o mutazioni; tale evento segnala errore del coordinamento.

Per InstallSnapshot: rivalidare termine, epoch e indice quando **entrambe** le barriere storage e applier sono quiescenti, dopo aver drenato o invalidato i vecchi lavori; se S è ormai <=lastApplied, non installare. Sincronizzare l'applier secondo [architettura](architecture.md), conservare hard state locale aggiornato, mantenere un suffisso soltanto con confine `(S,termS)` coincidente e mai scartare commit. Se lastApplied<S<=commitIndex e il confine non coincide, fail-closed. Pubblicare prima lo stato persistente, poi sostituire la mappa/epoch in RAM sotto barriera, infine rispondere installed_index e riaprire scheduling. Un crash fra pubblicazione e sostituzione RAM riparte dalla nuova generazione: non si inviano risposte usando la vecchia mappa.

T-016 deve introdurre un voto o un avanzamento di termine fra cattura a S e barriera finale, poi crash dopo switch: il voto aggiornato deve sopravvivere. T-015 deve avanzare apply durante il trasferimento e ritardare un completamento della vecchia applyEpoch.

## Livelli di durabilità e riduzione delle scritture

| Guasto | Contratto progettuale | Evidenza ancora necessaria |
|---|---|---|
| Crash del processo, OS vivo | Obiettivo v1: preservare successi con maggioranza recuperabile e I/O completato coerente; write non Sync possono sopravvivere. | T-013/T-016 e kill controllati T-023, non ancora eseguiti. |
| Crash/reboot del sistema operativo | Nessuna garanzia già validata; il protocollo Sync/rename è progettato per questo livello, subordinato al contratto macOS. | Fonti piattaforma e simulazioni non equivalgono a prove di crash del sistema reale. |
| Power loss / guasto SSD | Fuori dalla garanzia v1; dipende da cache, controller e dispositivo comune alle tre repliche. | Nessuna prova distruttiva richiesta o effettuata. |

Conservare Sync necessarie a voto, hard state, replica e pubblicazione snapshot. Non sincronizzare heartbeat identici senza variazioni persistenti; non riscrivere entry già identiche; checkpoint commit solo quando avanza. È consentito combinare hard state, append e commit appresi nello stesso frame quando l'ordine causale lo permette; non anticipare un ack in attesa del batch. Il checkpoint commit sincrono è una scelta iniziale conservativa, non un requisito universale di Raft: mantenerlo finché un ADR e test recovery giustifichino alternativa.

La GET nel log aggiunge entry e normalmente checkpoint su ciascuna replica che apprende commit, quindi I/O non nullo anche per workload read-only. Misurare bytes scritti e numero Sync per operazione logica in M15; preferire prima batching sicuro entro deadline, log diagnostici campionati e carichi piccoli. ReadIndex è la futura ottimizzazione mirata alle letture, secondo ADR-006; non eliminare sincronizzazioni necessarie per migliorare una misura.

La validazione finale dell’immagine confronta cluster/config, indice e termine inclusi, lunghezza e digest con l’envelope RPC; disaccordo è rifiuto, non installazione parziale. Il leader avanza matchIndex fino a S e nextIndex almeno S+1 soltanto su acknowledgment finale correlato `installed_index=S`, termine/generazione ancora validi e pubblicazione durevole conclusa; ack dei soli chunk non attestano replica. Un ack perso può essere risolto dal percorso snapshot obsoleto e dalla successiva AppendEntries verificata.

## Formato normativo e contratto piattaforma provvisorio

La [specifica canonica v1 con fixture verificate](canonical-formats.md) fissa ora lo schema storage: ordine ricorsivo ASCII dei campi, kind/nullabilità, digest comando tipizzato, oggetti entry/risultato/sessione, array/contatori snapshot e IDENTITY/CURRENT con checksum. Quel documento fornisce framing esatto e aspettative bootstrap/recovery; questa pagina conserva ordine e protocollo generazionale. Nessun codec storage esistente.

Il [contratto macOS](macos-storage-contract.md) registra il percorso Darwin effettivo File.Sync in Go 1.27.4, lock advisory stabile e requisito fsync directory da testare separatamente. Se rename CURRENT riesce ma la barriera directory fallisce, pubblicazione incerta/fatale: conservare entrambe le generazioni, niente rollback o ack. C-01 risolto come progetto; C-02 condizionato alle evidenze reali dell’adattatore. [Revisione corrente](pre-m03-review.md). Test storage e accettazione durabilità restano futuri.

## Confine dell’implementazione preliminare

Le [evidenze di fase](milestone-m03-preliminary.md) verificano contratto primitive locali C-02 e codec senza stato QWAL/file controllo. Nessun motore completo append/replay/generazione disponibile. PublishCurrent sincronizza un file controllo validato; il chiamante deve già aver sincronizzato/validato la generazione referenziata. Checksum frame o fsync riuscito non provano commit legale o correttezza recovery completa. M03 resta PARTIALLY COMPLETED; conservare tutti i gate storage/snapshot/durabilità successivi.

## WAL e recovery locali correnti — M03 completata

[L’implementazione locale completa con test](milestone-m03.md) offre ora bootstrap vuoto esplicito, coordinamento identità/CURRENT con checksum, append seriale Sync-prima-del-token e recovery streaming limitato hard-state/log/commit. Open ordinaria seleziona solo CURRENT, controlla identità/checkpoint e non usa fallback. Frame finale incompleto dopo prefisso iniziale interamente verificato troncato e sincronizzato; corruzione/violazione stato completa o errore lettura bloccano senza repair. Frame completi non-Sync recuperati risincronizzati prima dell’uso.

Recovery non applica comandi KV; chiamanti dovranno coordinare prova commit, sessioni/noop e applier. M03 supporta solo checkpoint vuoto obbligatorio; basi snapshot nonzero rifiutate esplicitamente fino a M13. AppendProposal riserva 512 KiB nel cap WAL 8 MiB; Append può usarli per replica/controllo. Tutti i file nodo conteggiati nei 32 MiB con margine crescita 64 KiB. Niente compaction/eliminazione per spazio: rifiutare al cap. Crash recovery/durabilità server completa resta M09, snapshot/rotazione generali M13. Precedenti stati progettuali/preliminari nel documento sono storici.

## Uso M06 transazioni suffisso esistenti

[M06](milestone-m06.md) riusa replace_suffix senza modificare codice storage o byte. Conserva prefisso committed, sostituisce dal primo conflitto non committed in transazione sincronizzata e installa RAM/ack solo dopo token correlato. Suffissi RPC leciti grandi divisi al cap byte entry per transazione esistente; crash può lasciare prefisso completo di questi frame. Nessun successo RPC finale prima di tutte le Sync richieste; recovery valida ogni frame intero sopravvissuto e retry riprende idempotentemente. Proposte locali usano AppendProposal; replica/controllo Append. commit_index noto invariato, nessun apply o nuova transizione checkpoint.

## Checkpoint e replay nuovo M07

[M07](milestone-m07.md) emette frame commit_index esistenti solo dopo entry coperte durevoli e prova leader/follower legale. Errore Sync ferma progresso/applicazione; recovery checkpoint completo sopravvissuto resta condizionato a validazione M03. Nessun file applied-index aggiunto. StartCommitted ricostruisce macchina memoria nuova con base zero tramite M02 su esattamente 1..commit recuperato, ignorando suffisso speculativo. Replay ricostruisce entrambi i cursori prima del listener. Recovery completo processi/MVP, sessioni e snapshot generali restano M09/M11/M13; perdita OS/alimentazione non verificata.

## Evidenze recovery processi M09

[M09](milestone-m09.md) completa sottoinsieme MVP interno T-014/T-018/T-023 con terminazione processi reale e replay su macchina nuova. Byte entry/checkpoint confermati recuperati da directory integre; ricostruzione M07 effettiva tramite M02 confrontata con mappa attesa esterna solo dopo stop owner. Shutdown ordinato attende lavoro prima di rilasciare porte/LOCK e scritture pendenti interrotte da segnale restano incerte. Inizializzazione mancante, lock duplicati, bootstrap ripetuto e corruzione completa falliscono chiusi prima di servire. Regole riparazione M03, riserve, byte canonici e limite base-zero invariati. AC-023 end-to-end GET completa resta dopo M10, pubblicazione/recovery snapshot M13. Durabilità crash/reboot OS e perdita alimentazione ancora non verificate.

## Replay read e costo limitato M10

[M10](milestone-m10.md) riusa comando read e byte digest v1 già approvati. GET riuscita richiede proposta sincronizzata ordinaria, quorum durevole e checkpoint commit; letture fallite/attesa abbandonata possono lasciare entry. Recovery nuova riesegue read committed nei propri slot esatti sulla macchina nuova, senza modificare KV o applicare suffissi speculativi. Test crash/restart reale a tre processi legge valore confermato e chiave assente tramite nuove barriere HTTP dopo recovery. Nessun risultato lettura è garanzia client/sessione persistita. Letture consumano cap/riserva WAL esistenti; compaction resta M13 e misure I/O M15. Nessun Sync rimosso o nuova garanzia OS/perdita alimentazione.

## Recovery sessioni versionate M11

[M11](milestone-m11.md) conserva byte transazioni/entry/digest WAL e introduce solo semantica applicativa configurazione v2/IDENTITY v2 esplicita, documentata in [ADR-010](adr/ADR-010-session-profile.md). Nuovo bootstrap v2 parte con mappa/sessioni vuote; recovery WAL base-zero esistente ricostruisce log/commit prima che M02 riesegua prefisso committed ricostruendo entrambi stati. Entry mutanti duplicate avanzano apply ma riusano risultato originale conservato. Suffissi non committed mai registrano sessioni durante startup. Commit legale nuovo termine successivo può registrarle esattamente una volta. Test reali crash/restart follower/leader/tutti processi conservano risultati CAS riusciti e falliti. Nessun file sessioni separato, cursore applied, conversione legacy implicita o rotazione snapshot. Cap WAL 8 MiB/directory 32 MiB invariati; anche retry conservato richiede spazio log nuovo e può essere indisponibile al cap. Limiti OS/alimentazione invariati.

## Modelli storage M12 senza nuovi formati

[M12](milestone-m12.md) aggiunge diagnostica locale e un simulatore esplicito separato dal server. Osservazioni e log non autorizzano commit, apply o risposte client e non cambiano gli algoritmi. Sessioni v2, risultato GET nello slot, CAS atomico e retry con nuovo quorum rimangono invariati; nessuna migrazione v1. Il simulatore usa core/M02/codec WAL reali, ghost committed permanente e confronto risultati per indice; replay confronta ogni evento con scelte salvate. ProcessCrash conserva anche scritture non sincronizzate completate; SystemCrashModel esplora sopravvivenza all/synced solo in memoria. Trace incomplete sono inconclusive. Nessun crash OS/alimentazione reale, snapshot o compattazione; gate M13/M14 conservati. Specifiche operative nei [fault](fault-injection.md) e nella [diagnostica](observability.md).
