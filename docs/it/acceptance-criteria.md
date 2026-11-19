# Criteri oggettivi di accettazione

[Indice](README.md) · [English](../en/acceptance-criteria.md)

**Aggiornamento: 19 novembre 2026 — M01 locale implementata e verificata; le specifiche distribuite restano progettuali.**

## Regola di accettazione

I criteri completi rimangono aperti; i sottoinsiemi locali M01 di T-008/T-009 e M02 di T-008/T-018 sono **Passed**, come documentato sotto. Per chiudere una riga servono procedura ripetibile, evidenza associata a revisione/configurazione e condizione rispettata. “Nessun panic” o un log senza errori non è sufficiente. Verifica il [catalogo test](testing-strategy.md) per dettagli di scheduling e fixture.

| Criterio | Requisito / procedura | Evidenza obbligatoria | Condizione di superamento |
|---|---|---|---|
| AC-001 | FR-001 / T-001. Avviare tre config distinte e tentare seconda apertura di n1. | Manifest e exit code. | Tre directory e tre identità distinte; seconda istanza rifiutata. |
| AC-002 | FR-002 / T-002. Variare membri, cluster ID e hash in config e RPC. | Fixture valide/invalide e stato prima/dopo. | Manifest divergente o membro duplicato impedisce avvio. |
| AC-003 | FR-003 / T-003. Iniettare JSON invalido, envelope errato e corpo oltre cap. | Risposte e hash stato invariato. | Payload malformati respinti senza mutare stato. |
| AC-004 | FR-004 / T-004. Simulare split vote e crash subito dopo voto Sync; rieseguire per 20 seed. | Traccia voti/termini/leader e seed. | Nessun termine con due leader in traccia, inclusi crash dopo voto. |
| AC-005 | FR-005 / T-005. Avanzare clock virtuale per 100 heartbeat senza guasti. | Conteggio elezioni e timer. | Leader stabile non produce elezioni spurie nel simulatore senza guasti. |
| AC-006 | FR-006 / T-006. Costruire log divergenti, inviare heartbeat e risposte obsolete, poi heal. | Prefissi, matchIndex e transizioni. | Log divergenti convergono senza sovrascrivere entry committed. |
| AC-007 | FR-007 / T-007. Riprodurre caso entry vecchia su maggioranza, poi noop corrente. | Commit invariato prima e avanzato dopo. | Entry vecchia non committed soltanto contando repliche. |
| AC-008 | FR-008 / T-008. Applicare stessa sequenza su tre modelli, ritardando un applier. | Digest canonico e risultati per indice. | Stessa sequenza produce stesso stato e risultato su tre nodi. |
| AC-009 | FR-009 / T-009. Enumerare chiave assente, valore vuoto, input invalido e DELETE ripetuto. | Tabella fixture API e modello. | Assenza, valore vuoto e DELETE ripetuto rispettano la tabella API. |
| AC-010 | FR-010 / T-010. Lanciare due CAS da sessioni diverse sullo stesso valore, new diversi. | Storia con un solo cambiamento riuscito. | Due CAS concorrenti stesso expected: al massimo uno cambia il valore. |
| AC-011 | FR-011 / T-011. Duplicare ultima sequenza, variare payload, usare sequenza vecchia e gap. | Risultati e tabella sessioni dopo replay. | Retry identico non applica nuovamente; cambio payload è errore. |
| AC-012 | FR-012 / T-012. Scartare risposta dopo apply, cambiare leader, ritentare identico. | Una sola mutazione e risultato originale. | Risposta persa dopo commit risolta da retry con stesso identificatore. |
| AC-013 | FR-013 / T-013. Tagliare frame a ogni confine; simulare short write/Sync error e CRC errato. | Immagini disco e decisione recovery. | Cutpoint prima/dopo Sync preservano voto e prefisso confermato. |
| AC-014 | FR-014 / T-014. Riavviare tutti con suffisso non committed e checkpoint commit inferiore. | Stato ricostruito prima e dopo nuovo noop. | Crash e restart non espongono suffisso non committed. |
| AC-015 | FR-015 / T-015. Installare snapshot a follower molto arretrato con retry chunk e cambio termine. | Hash immagine, sessioni e indici finali. | InstallSnapshot ripristina lo stesso stato e dedup. |
| AC-016 | FR-016 / T-016. Arrestare a ogni cutpoint di pubblicazione/cleanup generazione. | CURRENT e generazione recuperata per caso. | Crash a ogni fase lascia almeno una generazione recuperabile. |
| AC-017 | FR-017 / T-017. Eseguire elezione e timeout, ispezionare status e log senza valori utente. | Campione log redatto e snapshot metriche. | Status e log correlano elezione e richiesta senza valori. |
| AC-018 | FR-018 / T-018. Inviare shutdown durante RPC e persistenza; controllare worker e lock. | Durata, goroutine e recovery successivo. | Arresto entro 5 s o terminazione segnalata con recovery successivo. |
| AC-019 | FR-019 / T-019. Stabilire isolamento completo e scartare messaggi pendenti; poi scrivere sulla maggioranza e invocare nuova GET sul vecchio leader. | Storia che esclude lettura stale di successo. | Leader isolato non restituisce lettura locale come successo. |
| AC-020 | FR-020 / T-020. Riprodurre due volte scenario con stesso seed e clock virtuale. | Tracce eventi identiche byte per byte. | Due replay simulati hanno stessa sequenza di transizioni. |
| AC-021 | NFR-001 / T-021. Enumerare piccoli schedule con 3 nodi, 3 proposte e fino a 2 guasti. | Report invarianti e bound esplorato. | Zero violazioni in suite deterministica finita; nessuna pretesa di prova esaustiva. |
| AC-022 | NFR-002 / T-022. Verificare 20 storie fino a 12 operazioni con CAS, retry e failover. | Ordine testimone o controesempio ridotto. | Checker trova ordinamento legale per tutte le storie piccole previste. |
| AC-023 | NFR-003 / T-023. Confermare dati, crash di processo su uno e poi tutti, recuperare directory intatte. | Lista successi confrontata con GET successivi. | Nessun successo perso nei test crash di processo del modello. |
| AC-024 | NFR-004 / T-024. Fermare follower, riavviarlo e attendere catch-up; poi fermare leader lasciando due nodi sani. | Timeline elezione e ripresa entro target locale. | Dopo stabilizzazione, con storage/CPU disponibili e quote non sature, ripresa entro il target locale di 3 s. |
| AC-025 | NFR-005 / T-025. Eseguire benchmark limitato secondo profilo risorse. | Campioni grezzi e metodologia. | Report contiene campioni, errori, hardware e intervallo di misura. |
| AC-026 | NFR-006 / T-026. Raggiungere quota dataset, code e disco simulato senza carico massivo. | Errori, RSS, CPU e spazio massimo. | Quota produce errore deterministico; profilo risorse e spazio di picco registrati. |
| AC-027 | NFR-007 / T-027. Confrontare alberi linguistici, ID, blocchi tecnici e link; revisione semantica. | Rapporto revisione con limiti dichiarati. | Parità di file, identificatori e link verificata a ogni revisione. |
| AC-028 | NFR-008 / T-028. Iniettare fake clock, PRNG, transport e storage nello stesso core. | Replay regressione senza rete reale. | Una regressione si riproduce da manifest e seed salvati. |
| AC-029 | NFR-009 / T-029. Registrare go version/GOARCH e compilare sul Mac ARM64. | Output build e suite locale. | Build e test locali documentano toolchain e architettura. |
| AC-030 | NFR-010 / T-030. Provare bind 0.0.0.0, input gigante e permessi larghi. | Rifiuti e permessi effettivi. | Bind pubblico rifiutato, file privati, payload eccessivi respinti. |
| AC-031 | NFR-011 / T-031. Disabilitare Internet mantenendo loopback ed eseguire integrazione offline autonoma; demo presentata a M16. | Manifest dipendenze e traccia offline. | Con rete esterna disabilitata l’integrazione offline delle operazioni previste riesce; la presentazione finale è separata. |
| AC-032 | NFR-012 / T-032. Revisionare dipendenze e procedura su macchina già posseduta. | Inventario licenze, costi e prerequisiti. | Inventario dipendenze e procedura non richiedono pagamenti né account obbligatori. |

## Gate e deroghe

M09 chiude soltanto il sottoinsieme MVP della roadmap; M14 tutte le condizioni di correttezza; M15 il report risorse e performance; M16 demo, budget e consegna. Un test inconclusive resta aperto. Ogni deroga registra requisito, motivo e rischio in entrambe le lingue; non autorizza a pubblicizzare una garanzia non verificata. Nessuna deroga può introdurre spese o implementazione in questa fase.

## Sottocasi obbligatori aggiunti dall’audit

Gli AC restano Not run. AC-003/AC-009 richiedono parsing rigoroso e CAS massimo entro corpo 48 KiB; AC-008 include backlog di commit e coda piena senza doppio apply; AC-013/AC-016 includono write non Sync sopravvissute e hard state aggiornato nel passaggio generazione; AC-015 include applyEpoch e snapshot divenuto obsoleto durante il trasferimento; AC-019 distingue nuova GET dopo isolamento da risposta di read già committed; AC-022 richiede il controesempio con retry dopo completamento; AC-026 include prenotazione dei file di controllo.

I sottoinsiemi iniziali della [roadmap](implementation-roadmap.md) non chiudono questi criteri interi. AC-031 si verifica con integration offline autonoma a M14; la presentazione della demo a M16 non è un prerequisito circolare. Tutti i sottocasi devono comparire nelle evidenze del test primario, non sono nuovi requisiti non tracciati.

## Chiusura limitata di M01

M01 è COMPLETED nel solo sottoinsieme ufficiale: semantica locale, CAS sequenziale, validazione, quote, ownership dei byte e determinismo verificati con test reali. [Relazione M01](milestone-m01.md). Non si chiudono AC-008 distribuito, AC-009 HTTP/GET Raft, AC-010 concorrente, AC-011 dedup né AC-026 risorse cluster. Questo registro M01 non chiude criteri completi successivi; seguono le evidenze locali M02.

## Evidenze revisione M02 e gate pre-M03

Sottoinsiemi locali T-008/T-018 M02 Passed, riesaminati dal sorgente con 21 test e race mirato; AC completi server/distribuiti aperti. [Relazione pre-M03](pre-m03-review.md): progetto C-01 risolto, validazione implementativa C-02 aperta, CONDITIONALLY READY soltanto per test iniziali conformità/adattatore. AC-013 richiede che il futuro codec consumi [fixture statiche](canonical-formats.md) positive/negative, oltre a test write parziali/Sync/errori/cutpoint. AC-016 richiede test pubblicazione/handoff alla propria fase; AC-023 ancora evidenze reali processo/riavvio. Nessuno è chiuso da questa documentazione.

## Evidenze successive M03 preliminare

[Relazione di fase](milestone-m03-preliminary.md): GATE C-02 VERIFIED per primitive locali, M03 PARTIALLY COMPLETED. Passano 7 test storageformat/18 sottocasi e 13 funzioni test storagefs/16 sottocasi; superati race mirato e regressioni M01/M02 invariate. Ambito codec: frame/entry/digest/manifest senza stato; nessun motore WAL/replay o chiusura completa AC-013/AC-014/AC-023. Gate snapshot e successivi aperti. Cifre/stati precedenti descrivono la fase storica pertinente.
