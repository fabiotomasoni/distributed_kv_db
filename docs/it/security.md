# Modello di sicurezza locale

[Indice](README.md) · [English](../en/security.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Confine di fiducia

Prototipo per singolo utente in ambiente locale fidato, non pronto per produzione. Bind esclusivamente su indirizzi IP loopback espliciti; rifiutare `0.0.0.0`, `::` e hostname che risolvono fuori loopback. Nessun forwarding di porte, tunnel pubblico o discovery esterno. Config statica, owner e directory verificati prima dei listener.

Processi locali dello stesso utente possono impersonare client o peer: cluster_id e config_hash proteggono da errori di configurazione, **non autenticano** un avversario. Altri utenti locali e browser sono rischio residuo: niente CORS permissivo, accettare soltanto application/json sugli endpoint POST e validare Host sugli indirizzi configurati per limitare richieste accidentali e DNS rebinding. Queste misure non sostituiscono autenticazione.

## Controlli obbligatori

Validare UTF-8, JSON rigoroso, campi duplicati/sconosciuti, Base64, chiavi, numeri e dimensioni prima di allocazioni grandi o proposta. Cap API 48 KiB, peer 384 KiB, chunk snapshot 64 KiB raw; limitare connessioni, header (8 KiB), code e deadline. Rifiutare Content-Encoding diverso da identity; non decomprimere dati non fidati. Nessuna stringa utente diventa percorso file o comando shell.

Directory 0700/file 0600, no directory condivise o symlink per dati attivi. Il futuro lock/adattatore filesystem verifica owner e file regolari; non seguire link simbolici di configurazione dati senza scelta esplicita. Nessuna password o valore nei log; solo dataset sintetici nei test e nelle evidenze pubblicate. Non caricare directory WAL/snapshot su GitHub.

Il runtime limita nuove proposte quando code/dataset saturano, senza espellere dati o sessioni. Lo storage locale può fermarsi per disco pieno: non promettere resistenza a denial of service. Endpoint fault/profiling disabilitati salvo sessioni test esplicite su loopback. Errori restituiscono codici stabili senza stack trace o percorsi personali.

## Esclusioni e revisione

Autenticazione avanzata, autorizzazione multiutente, TLS/mTLS obbligatori e isolamento multi-tenant sono fuori MVP. Prima di qualunque uso fuori dal computer fidato occorrono threat model e ADR nuovi, autenticazione peer/client, TLS, gestione chiavi e audit: l'attuale progetto non autorizza esposizione pubblica.

Test T-030: bind pubblico negato; payload oversize/malformato senza stato mutato; permessi effettivi privati; nessun dato sensibile nei log; fault endpoint assente nel profilo normale. Dipendenze mantenute e revisionate secondo [guida](development-guide.md), senza scansioni cloud obbligatorie o servizi a pagamento.
