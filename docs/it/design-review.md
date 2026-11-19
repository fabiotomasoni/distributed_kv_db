# Revisione progettuale e questioni aperte

[Indice](README.md) · [English](../en/design-review.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Esito della revisione di progetto

Baseline documentale 0.1 pronta per revisione del proprietario. La fase resta esclusivamente documentale: nessun codice Go, CLI, server, deploy o benchmark. Gli ADR sono Proposed. Questa revisione non approva automaticamente la fase implementativa.

La revisione incrociata ha controllato: quorum 2/3 uniforme; commit solo mediante indice del termine corrente; voto e replica positivi dopo Sync; apply soltanto committed; GET mediante nuova entry read e lettura nello slot esatto; retry con sessione replicata, inclusa nello snapshot; compaction dopo pubblicazione durevole; limiti fisici comprensivi del picco temporaneo. Esempi API usano indici stringa come il protocollo peer. Il follower della demo è riavviato prima del crash del leader per lasciare una maggioranza.

## Questioni aperte e gate

| ID | Questione / baseline attuale | Evidenza necessaria / responsabile | Gate |
|---|---|---|---|
| OQ-01 | Sync directory, lock esclusivo e rename su macOS ARM64 | Sviluppatore: documentazione piattaforma, fixture recovery e cutpoint di processo; se primitive insufficienti rivedere ADR-005. | M03, prima di claim durabilità |
| OQ-02 | Resistenza a power loss APFS/SSD non verificata | Conservare garanzia limitata al modello; nessun test distruttivo sul Mac personale. Eventuale garanzia ulteriore richiede ricerca e evidenza separate. | Nessun claim power loss in v1 |
| OQ-03 | Timing 100/600–1000 ms e risorse sono ipotesi | Sviluppatore: baseline locale con Sync, RSS/CPU e picco disco; cambiare uniformemente config e documenti se necessario. | M05/M15 |
| OQ-04 | Licenza QuorumKV non selezionata | Proprietario: scelta esplicita prima di distribuzione pubblica. Nessun file LICENSE inventato in questa consegna. | Prima di pubblicazione |
| OQ-05 | Schema OpenAPI e fixture binarie/canoniche ancora da produrre | Sviluppatore: derivare contratti già descritti, validare esempi e formato con fixture byte-per-byte. | M03/M04/M08 |
| OQ-06 | Cap 1 024 sessioni e ultima risposta soltanto | Baseline intenzionale, nessuna eviction. Rivedere solo se demo o client previsti non la rispettano; mai TTL silenzioso. | M11 |
| OQ-07 | Piano 24 settimane e 6–8 ore/settimana incerto | Proprietario/sviluppatore: rivedere capacità dopo prime tre milestone; togliere opzionali o estendere calendario. | M03 e ogni gate |

## Verifiche documentali effettuate

Controlli automatici locali su presenza e parità degli alberi, link Markdown relativi, blocchi fenced bilanciati, JSON negli esempi, identificatori e corrispondenza di requisiti/AC/test nella matrice. Nove coppie di diagrammi Mermaid verificate nella struttura e nella sequenza; il diagramma sintetico è nel README principale. Il [rapporto di consegna](delivery-report.md) contiene l'inventario completo.

Revisione semantica manuale delle due lingue, dei casi limite e dei collegamenti fra API, Raft, storage e test. Fonti primarie consultate e registrate con data in [references](references.md). Gli esempi di comando sono dichiarati futuri, le misure Not measured e i criteri Not run.

## Limiti delle verifiche

I controlli automatici non dimostrano equivalenza semantica delle traduzioni o correttezza di un futuro algoritmo. Mermaid è stato controllato come sorgente e struttura, **senza rendering grafico o parser Mermaid dedicato**: tali strumenti non erano disponibili e non è stata installata infrastruttura aggiuntiva. Non è stato eseguito alcun test del database, checker di linearizzabilità, race detector o benchmark. Link esterni verificati per accessibilità al momento della consultazione, senza garanzia di disponibilità futura.

## Convenzioni da mantenere

Stesso cambiamento per entrambe le lingue; un requisito ritirato mantiene ID e motivazione, senza riuso. Un'API modificata aggiorna test, errori, ADR e tracce. Distingui documentazione verificata, proprietà progettata e proprietà testata. Il prossimo passo consigliato dopo approvazione è M01: modello sequenziale KV, fixture assenza/vuoto/CAS e quote, senza rete né ottimizzazioni.

## Registro storico della prima consegna 0.1

Controllo locale del 18 novembre 2026: **96 file Markdown**, **47 coppie linguistiche**, **895 collegamenti relativi risolti**, **14 blocchi JSON validi**, **13 coppie di esempi tecnici non Mermaid identiche**, **nove coppie di diagrammi con struttura corrispondente**. Cataloghi: 32 requisiti, 32 AC e 32 test previsti, tutti coperti dalla matrice. Esito dei controlli strutturali: **zero errori rilevati**. Questo esito riguarda documenti, non test applicativi.

## Esito dell’audit successivo 0.2

Il controllo strutturale iniziale non escludeva problemi semantici: l’[audit architetturale](architecture-review.md) registra 11 rilievi e correzioni, aggiornando il giudizio a **APPROVED WITH CONDITIONS**. Le cifre del registro precedente sono storiche, non la misura del repository attuale. OQ-01/OQ-05 sono gate prima di storage/protocolli durevoli; il consenso completo resta subordinato ai test. M01–M02 sono le sole milestone tecnicamente autorizzabili ora; nessuna implementazione viene iniziata con questo incarico.

## Decisioni correnti prima di M03

19 novembre 2026: [revisione e confine esatto di readiness](pre-m03-review.md). Schemi storage e fixture byte OQ-05/C-01 risolti; configurazione/protocollo M04 e OpenAPI M08 ancora pendenti. Fonti piattaforma e scelte provvisorie adattatore OQ-01/C-02 documentate; validazione reale directory/lock/rename/errori resta aperta, all’inizio tramite test isolati M03. Soltanto quel lavoro limitato conformità/capacità è condizionatamente ammissibile; integrazione storage attende evidenze, gate durabilità M03/M09 conservati. Nessun cambiamento OQ-02/03/04/06/07 o autorizzazione a implementare qui M03.
