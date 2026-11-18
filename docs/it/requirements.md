# Requisiti verificabili

[Indice](README.md) · [English](../en/requirements.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Regole normative

“Deve” identifica un requisito verificabile; “obiettivo” un valore ancora da misurare. Tutti i requisiti sono **Must** per la prima versione completa, salvo NFR-005 (Should: report obbligatorio, soglie da fissare dopo baseline). L'assegnazione a una milestone non è un risultato. I test e i criteri seguenti sono pianificati, non eseguiti.

La motivazione è esplicitata dal problema nella seconda colonna: identità evita alias di voto; ordine e quorum evitano divergenze; storage sostiene il modello crash; limiti e offline rendono il progetto realizzabile sull'hardware disponibile. Dettagli operativi: [API](api-specification.md), [Raft](raft-consensus.md), [storage](storage-and-recovery.md).

| ID | Requisito e motivazione | Priorità | Accettazione oggettiva | Verifica |
|---|---|---|---|---|
| FR-001 | **Avvio e identità**. Validare node_id univoco e directory esclusiva prima di aprire listener. **Motivo:** Evitare alias e accessi concorrenti allo stesso storage. | Must | Tre directory e tre identità distinte; seconda istanza rifiutata. | AC-001 / T-001 | 
| FR-002 | **Configurazione statica**. Accettare solo tre membri e stesso cluster_id e config_hash. **Motivo:** Evitare cluster incompatibili e maggioranze diverse. | Must | Manifest divergente o membro duplicato impedisce avvio. | AC-002 / T-002 | 
| FR-003 | **RPC peer**. Scambiare RPC versionate con identità validata e limiti. **Motivo:** Contenere input errati prima del consenso. | Must | Payload malformati respinti senza mutare stato. | AC-003 / T-003 | 
| FR-004 | **Elezione**. Eleggere con maggioranza e voto persistente unico per termine. **Motivo:** Impedire leader concorrenti nello stesso termine. | Must | Nessun termine con due leader in traccia, inclusi crash dopo voto. | AC-004 / T-004 | 
| FR-005 | **Heartbeat**. Inviare AppendEntries vuoti e timer senza busy loop. **Motivo:** Mantenere leadership stabile a riposo. | Must | Leader stabile non produce elezioni spurie nel simulatore senza guasti. | AC-005 / T-005 | 
| FR-006 | **Replica e conflitti**. Riconciliare suffissi non committed preservando prefissi. **Motivo:** Riparare divergenze senza perdere decisioni. | Must | Log divergenti convergono senza sovrascrivere entry committed. | AC-006 / T-006 | 
| FR-007 | **Commit**. Committare mediante matchIndex di maggioranza e termine corrente. **Motivo:** Impedire commit non sicuri dopo elezione. | Must | Entry vecchia non committed soltanto contando repliche. | AC-007 / T-007 | 
| FR-008 | **Apply ordinato**. Applicare una volta per indice il prefisso committed in ciascuna ricostruzione della macchina a stati; replay dopo restart non è un nuovo effetto logico. **Motivo:** Ottenere repliche deterministiche. | Must | Stessa sequenza produce stesso stato e risultato su tre nodi. | AC-008 / T-008 | 
| FR-009 | **PUT/GET/DELETE**. Esporre operazioni complete su chiavi e valori validi. **Motivo:** Offrire un contratto client non ambiguo. | Must | Assenza, valore vuoto e DELETE ripetuto rispettano la tabella API. | AC-009 / T-009 | 
| FR-010 | **CAS atomico**. Valutare confronto e aggiornamento come singolo comando applicato. **Motivo:** Evitare lost update su una chiave. | Must | Due CAS concorrenti stesso expected: al massimo uno cambia il valore. | AC-010 / T-010 | 
| FR-011 | **Sessioni client**. Serializzare sequenze client e conservare ultimo risultato replicato. **Motivo:** Evitare effetti duplicati. | Must | Retry identico non applica nuovamente; cambio payload è errore. | AC-011 / T-011 | 
| FR-012 | **Deadline e retry**. Classificare timeout dopo ammissione come esito incerto. **Motivo:** Non confondere assenza di risposta e rollback. | Must | Risposta persa dopo commit risolta da retry con stesso identificatore. | AC-012 / T-012 | 
| FR-013 | **WAL durevole**. Sincronizzare hard state e log prima di risposte Raft positive. **Motivo:** Rendere affidabili i voti e le repliche confermate. | Must | Cutpoint prima/dopo Sync preservano voto e prefisso confermato. | AC-013 / T-013 | 
| FR-014 | **Restart**. Ripartire follower ricostruendo soltanto stato committed provato. **Motivo:** Non esporre comandi speculativi dopo crash. | Must | Crash e restart non espongono suffisso non committed. | AC-014 / T-014 | 
| FR-015 | **Snapshot**. Includere mappa, sessioni, indice, termine e configurazione. **Motivo:** Recuperare anche peer senza il vecchio log. | Must | InstallSnapshot ripristina lo stesso stato e dedup. | AC-015 / T-015 | 
| FR-016 | **Compaction**. Eliminare vecchio WAL solo dopo pubblicazione durevole snapshot. **Motivo:** Limitare crescita senza eliminare dati necessari. | Must | Crash a ogni fase lascia almeno una generazione recuperabile. | AC-016 / T-016 | 
| FR-017 | **Diagnostica**. Esporre ruolo, indici e log senza dati utente. **Motivo:** Rendere i guasti spiegabili senza divulgare valori. | Must | Status e log correlano elezione e richiesta senza valori. | AC-017 / T-017 | 
| FR-018 | **Shutdown**. Fermare ammissione, chiudere worker e sincronizzare storage. **Motivo:** Evitare risorse orfane e arresti incoerenti. | Must | Arresto entro 5 s o terminazione segnalata con recovery successivo. | AC-018 / T-018 | 
| FR-019 | **Letture linearizzabili**. Completare GET solo dopo barrier corrente committed e applicata. **Motivo:** Bloccare letture obsolete dalla minoranza. | Must | Leader isolato non restituisce lettura locale come successo. | AC-019 / T-019 | 
| FR-020 | **Fault injection**. Riprodurre eventi rete, storage e processo con seed e traccia. **Motivo:** Riprodurre guasti rari. | Must | Due replay simulati hanno stessa sequenza di transizioni. | AC-020 / T-020 | 
| NFR-001 | **Safety**. Preservare Election Safety, Log Matching, Leader Completeness e State Machine Safety. **Motivo:** Proteggere il significato del consenso. | Must | Zero violazioni in suite deterministica finita; nessuna pretesa di prova esaustiva. | AC-021 / T-021 | 
| NFR-002 | **Linearizzabilità**. Rispettare ordine reale delle operazioni completate. **Motivo:** Rispettare ordine reale percepito dai client. | Must | Checker trova ordinamento legale per tutte le storie piccole previste. | AC-022 / T-022 | 
| NFR-003 | **Durabilità condizionata**. Conservare successi con maggioranza persistente recuperabile. **Motivo:** Conservare successi entro il modello dichiarato. | Must | Nessun successo perso nei test crash di processo del modello. | AC-023 / T-023 | 
| NFR-004 | **Tolleranza a un crash**. Progredire con due nodi sani e rete tempestiva. **Motivo:** Dimostrare disponibilità con una replica guasta. | Must | Dopo stabilizzazione, con storage/CPU disponibili e quote non sature, ripresa entro il target locale di 3 s. | AC-024 / T-024 | 
| NFR-005 | **Performance misurabile**. Misurare throughput, media, p95/p99 senza soglie inventate. **Motivo:** Confrontare modifiche usando evidenze. | Should | Report contiene campioni, errori, hardware e intervallo di misura. | AC-025 / T-025 | 
| NFR-006 | **Risorse limitate**. Rispettare cap logici, code e disco; misurare obiettivi RAM/CPU. **Motivo:** Proteggere il Mac già posseduto. | Must | Quota produce errore deterministico; profilo risorse e spazio di picco registrati. | AC-026 / T-026 | 
| NFR-007 | **Manutenibilità bilingue**. Mantenere documenti paralleli e decisioni tracciabili. **Motivo:** Permettere manutenzione senza divergenze linguistiche. | Must | Parità di file, identificatori e link verificata a ogni revisione. | AC-027 / T-027 | 
| NFR-008 | **Testabilità**. Iniettare tempo, random, rete e storage senza servizi esterni. **Motivo:** Separare core e ambiente reale. | Must | Una regressione si riproduce da manifest e seed salvati. | AC-028 / T-028 | 
| NFR-009 | **Portabilità ARM64**. Usare Go e libreria standard su darwin/arm64. **Motivo:** Evitare toolchain o hardware aggiuntivi. | Must | Build e test locali documentano toolchain e architettura. | AC-029 / T-029 | 
| NFR-010 | **Sicurezza locale**. Limitare bind a loopback, input e permessi. **Motivo:** Limitare esposizione accidentale e input ostili. | Must | Bind pubblico rifiutato, file privati, payload eccessivi respinti. | AC-030 / T-030 | 
| NFR-011 | **Offline**. Eseguire cluster e suite senza Internet dopo bootstrap. **Motivo:** Non dipendere da servizi esterni per funzionare. | Must | Con rete esterna disabilitata l’integrazione offline delle operazioni previste riesce; la presentazione finale è separata. | AC-031 / T-031 | 
| NFR-012 | **Zero spese**. Non richiedere servizi, acquisti o infrastrutture a pagamento. **Motivo:** Rispettare il vincolo economico assoluto. | Must | Inventario dipendenze e procedura non richiedono pagamenti né account obbligatori. | AC-032 / T-032 | 

La [matrice](traceability.md) associa ogni requisito a modulo, ADR, milestone e test. Nessun requisito può essere chiuso con la sola ispezione di un diagramma. Per NFR-001–003 i test costituiscono evidenza finita, non dimostrazioni universali.
