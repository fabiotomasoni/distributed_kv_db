# Panoramica di QuorumKV

[Indice](README.md) · [English](../en/overview.md)

**Stato: progetto pre-implementativo, revisione 0.1 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Scopo e motivazione

QuorumKV sarà un database key-value distribuito scritto in Go da zero: una mappa da chiavi a sequenze di byte, replicata da tre processi indipendenti. È un progetto didattico e di portfolio per uno sviluppatore individuale, pensato per rendere osservabili consenso, concorrenza, durabilità e recupero. Questo repository contiene soltanto il progetto: non esistono ancora server, CLI, test eseguibili o benchmark.

Un database key-value recupera un valore intero tramite una chiave; non interpreta proprietà interne, tabelle o join. Distribuirlo significa mantenere copie su nodi che comunicano con messaggi, senza assumere che abbiano la stessa memoria, lo stesso stato o una rete sempre disponibile. Il problema principale è stabilire un ordine concordato degli aggiornamenti nonostante crash e ritardi.

## Tre repliche, una macchina logica

Un solo gruppo Raft avrà tre membri statici: `n1`, `n2`, `n3`. La maggioranza è `floor(3/2)+1 = 2`. Due maggioranze si intersecano: questo fatto, insieme a voto persistente, filtro elettorale e regola di commit, sostiene la sicurezza. La sola intersezione non prova la correttezza dell'implementazione.

Il leader propone comandi in un log ordinato; ogni replica applica soltanto il prefisso committed, nello stesso ordine, a una macchina a stati deterministica. Il risultato logico dovrà essere quello di una singola mappa. Raft separa elezioni, replica e sicurezza in sottoproblemi ispezionabili: è adatto all'obiettivo di apprendimento, ma verrà implementato direttamente senza librerie di consenso.

La maggioranza può continuare con un processo fermo, se rete, storage e scheduling permettono progresso. Due nodi indisponibili impediscono nuove operazioni linearizzabili. I tre processi condividono computer, alimentazione e SSD: non offrono indipendenza fisica né alta disponibilità del servizio rispetto al guasto del Mac.

## Funzioni previste e utilizzi

Sono previsti `PUT`, `GET`, `DELETE`, `CAS`, persistenza WAL, restart, snapshot, diagnostica e fault injection locale. Esempi: configurazioni sintetiche, inventario limitato, metadata di risorse, contatori aggiornati mediante CAS. Non usarlo per credenziali, pagamenti, dati insostituibili o carichi di produzione.

| Sistema | Differenza rispetto all'obiettivo di QuorumKV |
|---|---|
| PostgreSQL | Modello relazionale, SQL e transazioni: fuori dall'ambito di questa mappa a singola chiave. |
| Redis | Ecosistema di strutture dati e funzionalità operative: qui si studiano soltanto quattro operazioni e consenso. |
| etcd | Riferimento maturo per un KV basato su Raft; non è una dipendenza né una promessa di compatibilità o equivalenza. |

Il valore del portfolio sarà la qualità delle evidenze: tracce di elezioni, storie concorrenti controllate, cutpoint di crash, decisioni motivate e limiti dichiarati. Il numero di feature non sostituisce queste prove.

## Percorso di lettura

Partire da [requisiti](requirements.md), [ambito](scope.md) e [architettura](architecture.md). Per una revisione di correttezza leggere insieme [Raft](raft-consensus.md), [consistenza](consistency-model.md), [storage](storage-and-recovery.md) e [test](testing-strategy.md). La [roadmap](implementation-roadmap.md) descrive il passaggio futuro dal progetto alle evidenze.
