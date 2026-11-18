# Ambito, assunzioni e limiti

[Indice](README.md) · [English](../en/scope.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Confini

L'obiettivo finale della prima versione è un gruppo Raft statico di tre nodi su loopback, con quattro operazioni atomiche su singola chiave, WAL sincrono, recovery e snapshot. Ogni nodo conserva l'intero dataset. `cluster_id`, membri, indirizzi e limiti deterministici sono identici per tutti; il processo non si avvia se la configurazione contraddice il manifest persistente.

## Incrementi e primo MVP

Il primo **MVP interno** è il gate M09: elezione, replica, commit, PUT/DELETE, WAL e recovery, esercitati da un client di test. Non è la prima versione completa e non espone ancora GET come lettura garantita. M10 aggiunge GET linearizzabile; M11 CAS e sessioni/retry; M13 snapshot e compaction; M14 chiude il gate di correttezza della prima versione. Non dichiarare completato il requisito di un'estensione al gate MVP precedente.

Dopo la prima versione si potranno valutare ReadIndex al posto delle letture nel log, batching del WAL e miglioramenti diagnostici. Sono opzionali e richiedono nuovi ADR ed evidenze. Pre-vote e check-quorum non sono necessari alla safety e restano estensioni; l'assenza di pre-vote può causare termini ed elezioni inutili dopo un isolamento.

## Esclusioni

Nessuno sharding, membership dinamica, SQL, transazioni multi-key, replica geografica, hosting pubblico, multi-tenancy, Kubernetes o compatibilità commerciale. Nessun motore KV o Raft esterno. Nessuna autenticazione avanzata, autorizzazione multiutente o TLS obbligatorio nel prototipo locale. Nessun servizio a pagamento, carta di credito, trial, dominio, hardware aggiuntivo o dipendenza runtime da Internet.

## Assunzioni operative

Fault crash-stop e crash-recovery con processi non malevoli; messaggi possono essere persi, duplicati, ritardati o riordinati. La safety non dipende da orologi sincronizzati. La liveness richiede una maggioranza funzionante e periodi sufficientemente lunghi di consegna tempestiva. Storage persistente affidabile secondo il contratto di [recovery](storage-and-recovery.md); corruzione e perdita simultanea del disco condiviso non sono mascherate da Raft.

Ambiente fidato, macOS ARM64 su MacBook Pro M5 con 32 GB. Budget aggiuntivo 0 €; energia e connessione già disponibili non sono nuove spese, ma non sono fisicamente gratuite. Le dipendenze si acquisiscono una volta, poi database e test devono funzionare offline. GitHub Free è opzionale per pubblicare il portfolio, mai necessario per eseguire il sistema.

## Limiti dichiarati

Risorse, timing e durabilità su power loss sono ipotesi da misurare, non risultati. I limiti iniziali sono normati in [risorse](performance-and-resources.md). La sicurezza di rete vale solo per loopback fidato. La licenza del repository è **da scegliere dal proprietario**; non si presume una concessione open source. La [revisione](design-review.md) registra i gate ancora aperti.
