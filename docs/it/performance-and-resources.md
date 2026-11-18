# Prestazioni e protezione delle risorse

[Indice](README.md) · [English](../en/performance-and-resources.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Profilo iniziale, non risultati

MacBook Pro M5, 32 GB, macOS ARM64; tre processi, nessuna GPU, training AI, infrastruttura pesante o servizio continuativo. I numeri seguenti sono limiti progettuali oppure obiettivi da misurare. Nessun consumo o throughput è stato misurato. MB è decimale, MiB = 1 048 576 byte: “circa 100 MB” è un obiettivo operativo approssimativo, non uguaglianza con 100 MiB.

| Risorsa | Limite / obiettivo iniziale | Enforcement / verifica futura |
|---|---|---|
| Dataset logico | 4 MiB e 10 000 chiavi per copia; 12 MiB tre copie | Quota deterministica in apply, T-026. |
| Valore / chiave | 16 KiB / 256 byte | Parser e modello, T-009. |
| Sessioni | 1 024; ultimo risultato <=512 byte | Nessuna eviction, incluso negli snapshot. |
| WAL fisico | 8 MiB/nodo, riserva interna 512 KiB | Ammissione/compaction, storage stop prudenziale. |
| Snapshot | 8 MiB/file | Lunghezza validata prima di allocare. |
| Directory dati | 32 MiB/nodo incluso picco temporanei e generazioni | Prenotazione prima di snapshot/install. |
| Dati cluster ordinari | Obiettivo <=70 MiB (~73 MB); picco cap 96 MiB (~101 MB) | Sommare tre directory, non una sola copia. |
| Log operativi | 1 MiB/nodo; 3 MiB cluster | Rotazione inclusiva, no debug continuo. |
| Dati+log attivi | Cap 99 MiB (~104 MB) | Stop prima di superamento; target ordinario circa 100 MB. |
| Evidenze test | 10 MiB separati dai dati attivi | Conservare solo fallimenti selezionati; retention manuale. |
| RAM cluster | Obiettivo RSS totale <1 000 MB | Misura macOS, non garanzia del runtime. |
| Heap Go | Target morbido 192 MiB/nodo | Futuro GOMEMLIMIT, non cap RSS né protezione da OOM. |
| CPU | Prevalentemente inattiva a riposo; GOMAXPROCS=2 per nodo iniziale | Timer bloccanti, nessun polling attivo. |
| Client ordinari | Massimo 8, 1 000 operazioni totali/run | Harness ferma nuove operazioni al cap. |
| Benchmark | 2 s warm-up +10 s misura, una ripetizione iniziale | Deadline 15 s; opzionale, non continuo. |
| Suite ordinaria | Massimo 120 s, integration <=30 s/caso | Timeout e cleanup obbligatori. |

Cap dati di 32 MiB include **tutti** i file temporanei e di controllo. Quattro file da 8 MiB consumerebbero già l'intero cap, senza spazio per IDENTITY, CURRENT, lock e manifest: non sono un picco ammissibile. Non avviare install snapshot nello stesso periodo. I limiti rendono più probabile rifiutare lavoro che esaurire il disco, ma l'overhead filesystem e altri programmi restano fuori dal cap applicativo. Se manca spazio fisico, fail-closed.

## Misurazione controllata

Usare dataset sintetico da 1 000 chiavi, valori 128 byte, pre-caricato entro quota; workload separati GET-only, PUT-only e 80% GET/20% PUT. Il mix GET comporta WAL nel progetto iniziale: specificarlo nel report. Concorrenza 1, poi 4, infine 8 soltanto se i run precedenti rispettano limiti. Nessuna ricerca automatica della saturazione; fermarsi in caso di pressione memoria, swap in crescita, thermal pressure o spazio oltre cap.

Throughput = operazioni completed nella finestra / secondi monotoni; distinguere tentativi HTTP e operazioni logiche deduplicate. Latenza end-to-end da prima invocazione a risposta, retry incluso; riportare separatamente errori, unknown e latenza dei soli successi per non nascondere timeout. Media sui campioni; p95/p99 nearest-rank su campioni ordinati, esplicitare n e bassa affidabilità del p99 con campioni piccoli. Non interpolare numeri mancanti né promettere SLO prima della baseline.

Misurare elezione come perdita introdotta→noop committed; target locale iniziale <=3 s dopo stabilizzazione. Recovery processo come avvio→replay completato; catch-up e prima risposta client a parte. Riportare dimensioni WAL/snapshot, numero entry e condizioni cache. Non svuotare cache del sistema o forzare reboot per rendere “puliti” i risultati.

## Monitoraggio su macOS

Monitoraggio Attività: selezionare i tre processi, osservare CPU, Memoria, Energia e byte scritti nel pannello Disco; confrontare con baseline a cluster fermo. `top -l 2 -pid <PID>` e `ps -o pid,rss,%cpu,etime -p <PID>` sono esempi di osservazione da usare con PID reali del proprio run, non comandi già applicati. RSS di ps è in KiB su macOS; sommare i tre processi evitando di interpretare memoria condivisa come misura perfetta.

`du -sk .local/n1 .local/n2 .local/n3` è esempio futuro per spazio allocato; confrontare con conteggio file logico dell'app, poiché APFS può allocare diversamente. Non usare strumenti privilegiati o campionamento ad alta frequenza. Per goroutine/heap usare runtime metrics locali; profiling opzionale su loopback solo durante sessione breve e disabilitato normalmente.

## Report e cleanup

Ogni report futuro contiene data, revisione, Go/macOS, hardware, alimentazione, configurazione, dataset, warm-up, durata, seed, concorrenti, n, errori, throughput, media/p95/p99, RSS/CPU picco e idle, spazio prima/picco/dopo, elezione/recovery e limiti. Tabella misurazioni attuale: **Not measured** per tutte le metriche.

A fine sessione fermare i processi, verificare PID e lock, conservare manifest ed evidenze essenziali; rimuovere solo directory di run esplicitamente riconosciute. Nessun cleanup automatico di dati attivi o file fuori dal progetto. Preferire compaction a soglia e carichi brevi: non eseguire benchmark ripetuti senza una domanda ingegneristica nuova.

## Prenotazione del picco e pressione delle quote

Prima di avviare rotazione o installazione, verificare `file_bytes_attuali + crescita_temporanea_prevista + riserva_controllo <= 32 MiB`. Riserva controllo iniziale **64 KiB**, compresa nel cap; se i file di controllo richiedono più spazio, usare la stima maggiore. Non allocare un secondo staging completo se si può promuovere il file ricevuto nella nuova generazione. La prenotazione deve includere append ammesse prima della barriera e ogni vecchia generazione ancora referenziata.

Se non entra, rimandare la compaction e bloccare nuove proposte prima del cap, senza cancellare il prefisso necessario. Può essere necessario attendere commit/catch-up per ridurre il suffisso vivo: nessuna garanzia di liveness sotto quota fisica esaurita. Il target di ripresa <=3 s presuppone anche risorse disponibili e assenza di quota saturata. Spazio allocato APFS va misurato separatamente dai byte logici; non promettere un limite fisico perfetto da soli contatori applicativi. T-026 simula il rifiuto prenotazione vicino al cap con dati di controllo presenti.
