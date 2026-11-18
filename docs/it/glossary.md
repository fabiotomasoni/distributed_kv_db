# Glossario bilingue

[Indice](README.md) · [English](../en/glossary.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Terminologia condivisa

Identificatori, nomi RPC e codici errore restano identici nelle due lingue. “Safety” indica correttezza distribuita, “security” controllo degli accessi: non sono sinonimi.

| English | Italiano | Definizione |
|---|---|---|
| Raft term | Termine Raft | Epoca logica monotona delle elezioni, non tempo fisico. |
| Quorum | Quorum | Maggioranza di 2 su 3 membri statici. |
| Leader | Leader | Nodo eletto che propone e replica entry nel proprio termine. |
| Follower | Follower | Replica che riceve log e può votare. |
| Candidate | Candidato | Nodo che richiede voti per il termine corrente. |
| Log entry | Entry di log | Comando con indice globale e termine di origine. |
| Commit | Commit | Decisione di consenso che rende il prefisso immutabile nel modello. |
| Apply | Applicazione | Esecuzione deterministica di un comando committed. |
| Linearizability | Linearizzabilità | Equivalenza a ordine sequenziale legale che rispetta ordine reale. |
| Read barrier | Barriera di lettura | Entry read corrente, committed e campionata nel suo slot apply. |
| WAL | Write-ahead log | Registro persistente da sincronizzare prima di acknowledgment dipendenti. |
| Durability | Durabilità | Conservazione condizionata al modello di guasto e storage. |
| Snapshot | Snapshot | Immagine consistente di dati e sessioni a indice committed noto. |
| Compaction | Compaction | Rimozione sicura della storia coperta da snapshot durevole. |
| CAS | Compare-and-swap | Confronto byte e aggiornamento atomici di una chiave. |
| ABA | Problema ABA | Valore torna uguale dopo modifiche intermedie, invisibili a CAS su valore. |
| Deduplication | Deduplicazione | Riconoscimento di una stessa identità per evitare ulteriori effetti. |
| Unknown outcome | Esito incerto | Mancanza di certezza client: la richiesta potrebbe essere committed. |
| Safety | Sicurezza algoritmica | Non accade uno stato proibito, anche senza progresso. |
| Liveness | Progresso | Operazioni avanzano sotto assunzioni di consegna/scheduling. |
| Crash-stop | Crash-stop | Processo si arresta senza rientrare nello scenario. |
| Crash-recovery | Crash-recovery | Processo si riavvia dallo stato persistente. |
| Byzantine fault | Guasto bizantino | Comportamento arbitrario o malevolo, escluso. |
| Backpressure | Backpressure | Rallentamento/rifiuto ammissione per code e risorse limitate. |
| Seed | Seed | Stato iniziale casualità riproducibile, insufficiente da solo per scheduling OS. |
| RSS | Memoria residente | Memoria fisica del processo secondo OS, diversa dal solo heap Go. |
| fsync / Sync | Sincronizzazione | Richiesta di persistenza al sistema, con garanzie dipendenti dal livello sottostante. |
