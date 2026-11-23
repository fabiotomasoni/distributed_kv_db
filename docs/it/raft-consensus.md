# Specifica del core Raft

[Indice](README.md) · [English](../en/raft-consensus.md)

**20 novembre 2026 — elezioni, replica e commit/applicazione M05–M07 implementati e verificati; safety client/snapshot completa futura.**

## Stato, indici e proprietà

Contratto di base: [paper Raft](https://raft.github.io/raft.pdf), figura 2 e sezioni 5–7. Questa specifica sceglie un gruppo statico e non implementa joint consensus. Nessuna proprietà è ancora dimostrata per QuorumKV.

Persistente prima di risposte dipendenti: `currentTerm`, `votedFor` (vuoto o un membro), log e metadati snapshot. Volatile: ruolo, leader hint, `commitIndex`, `lastApplied`; leader: `nextIndex[peer]`, `matchIndex[peer]`, richieste attive. `snapshotIndex <= lastApplied <= commitIndex <= lastLogIndex`; dopo recovery lastApplied parte dallo snapshot e cresce per replay del commit persistito. Il commit persistito è un'estensione locale prudenziale descritta nello [storage](storage-and-recovery.md).

Il log usa indici globali contigui uint64, inizio 1; `(0,0)` è sentinella. Lo snapshot conserva `(lastIncludedIndex,lastIncludedTerm)` come confine confrontabile. Il termine dell'ultima entry per l'elezione include tale confine quando il suffisso è vuoto.

| Invariante | Obbligo progettuale e argomento |
|---|---|
| INV-01 Election Safety | Al massimo un leader per termine: maggioranze intersecanti e un voto persistente per termine. |
| INV-02 Log Matching | Stesso indice e termine implica prefisso identico: controllo prev e sostituzione soltanto al primo conflitto. |
| INV-03 Leader Completeness | Ogni leader futuro contiene le entry committed: filtro log up-to-date e regola di commit corrente; non segue dal solo quorum. |
| INV-04 State Machine Safety | Nessuna replica applica comandi diversi allo stesso indice: prefisso committed immutabile e apply ordinato. |
| INV-05 Monotonicità | Termine, commit e apply non regrediscono; matchIndex non regredisce nella stessa generazione leader. |
| INV-06 Persist-before-ack | Voto e replica positiva rappresentano stato già sincronizzato. |

## Transizioni e timer

Avvio sempre follower, voto recuperato e nessuna presunzione di leadership precedente. Timeout elettorale: incrementare termine, diventare candidate, votare sé stessi, persistere, quindi inviare RequestVote. Contare ogni membro una sola volta, sé stesso solo dopo Sync; con due voti dello stesso termine diventare leader. Reinizializzare nextIndex=lastLogIndex+1 e matchIndex peer=0, locale=ultimo indice durevole. Appendere un noop del nuovo termine; nessun successo client dipende da una vecchia entry prima di commit legale.

Qualunque RPC o risposta valida con termine maggiore: aggiornare termine, azzerare votedFor, diventare follower e sincronizzare prima di risposta. Termine inferiore: rifiuto con termine corrente, nessun reset timer. Candidate riceve AppendEntries valido di termine uguale: step-down senza azzerare voto del termine. Due leader distinti stesso termine sono violazione INV-01: registrare e fallire nel simulatore, non inventare una regola di fusione.

Resettare timeout solo iniziando elezione, concedendo voto o ricevendo AppendEntries/InstallSnapshot di leader riconosciuto con termine almeno corrente (anche se prev non coincide). Non resettare su RequestVote negato né su traffico qualsiasi. In split vote nessuno vince, timeout casuali nuovi evitano sincronizzazione sistematica; nessun limite teorico di tempo in rete asincrona.

## RequestVote

Concedere se termine non obsoleto, `(votedFor è vuoto oppure candidate_id)` e log candidato almeno aggiornato: confrontare lessicograficamente `(lastLogTerm,lastLogIndex)`, prima termine poi indice. Persistenza di termine e voto prima di `vote_granted=true`; un voto ripetuto per lo stesso candidato può essere confermato. Un crash subito dopo voto non permette voto diverso nello stesso termine. Risposte vecchie non si contano nella nuova elezione; termine maggiore impone step-down.

## AppendEntries, heartbeat e conflitti

1. Validare envelope e batch senza effetti parziali; gestire termine e ruolo.
2. Verificare che prev esista con prevLogTerm, inclusa sentinella snapshot; se manca, rifiutare con indice utile. Se prev è prima del confine snapshot, richiedere riallineamento al confine/installazione, senza accessi a indici eliminati.
3. Cercare il primo indice con termine diverso. Troncare il suffisso **soltanto dal conflitto** e appendere le entry mancanti. Non troncare perché il batch finisce prima del log locale; heartbeat non elimina entry. Se conflitto è <= commitIndex, fermarsi con errore di integrità: non sovrascrivere committed.
4. Persistenza atomica della sostituzione di suffisso prima del successo. Per entry già identiche nessuna riscrittura necessaria, salvo hard state pendente.
5. Aggiornare commit a `max(commitIndex, min(leaderCommit, prevLogIndex+len(entries)))`, ossia non oltre il prefisso verificato da questa richiesta; persistere avanzamento prima di apply.
6. Rispondere success con match_index=prev+len(entries), non con tutta la lunghezza del log locale.

Su rifiuto leader riduce nextIndex secondo conflict_index; se conflict_term è noto localmente può saltare all'ultimo indice di quel termine +1. Hint sempre limitati al log corrente e alla richiesta correlata. Se nextIndex <= snapshotIndex, inviare snapshot. Su successo aggiornare matchIndex monotonicamente e nextIndex=match+1. Una risposta di fallimento obsoleta non annulla un successo più recente; una RPC per peer semplifica ma non elimina la necessità di correlazione.

## Commit e apply

Il leader sceglie il massimo N con `N > commitIndex`, `log[N].term == currentTerm` e almeno due `matchIndex >= N` includendo il proprio log durevole. Solo allora può portare commitIndex a N e includere transitivamente tutto il prefisso. Non committare un'entry di vecchio termine solo perché ora è presente su due nodi. Il noop corrente permette di rendere committed indirettamente il prefisso ereditato.

Pseudocodice progettuale, non implementazione:

```text
on durable_replicated_progress:
  N := largest index with majority durable match and term[N] = currentTerm
  if N > commitIndex:
    persist(commit=N)
    after durable: commitIndex := N; schedule_apply()
on schedule_apply or apply_capacity_available:
  end := min(commitIndex, scheduledApplyIndex + available_apply_slots)
  if end > scheduledApplyIndex:
    enqueue_apply(scheduledApplyIndex+1 .. end, applyEpoch)
    scheduledApplyIndex := end  // only after successful queue admission
on applied(epoch, i, result):
  if epoch != applyEpoch: discard obsolete completion; return
  require i = lastApplied+1
  require i <= scheduledApplyIndex <= commitIndex
  lastApplied := i
  resolve matching client waiter only if response conditions hold
  schedule_apply()
```

Applicare contiguamente e senza applicare speculative entry. Risultati mutanti e sessioni cambiano nella stessa transizione deterministica. Durante leadership change i waiter possono ricevere esito incerto; nessuna entry non committed viene presentata come definitiva.

## Casi limite e liveness

| Caso | Comportamento obbligatorio |
|---|---|
| Leader perde quorum | Può ancora credersi leader; non può committare né rispondere con GET locale. Deadline → unknown. |
| Vecchio leader ritorna | Termini maggiori lo rendono follower; suffisso non committed può essere sostituito. |
| Log divergenti | Confronto prev e riparazione suffisso; committed non viene rimosso. |
| Leader eredita entry vecchie | Committare noop/entry corrente prima del commit per conteggio del prefisso. |
| Risposte tardive | Ignorare progressi di generazioni obsolete; osservare comunque termini maggiori validi. |
| Crash dopo voto/append | Recovery mantiene stato sincronizzato; mai acknowledgment precedente a Sync. |
| Nuovo leader con log incompleto | Filtro elettorale vieta vincita se escluderebbe prefisso committed; entry non committed possono sparire. |

Il sistema privilegia safety alla disponibilità. Non usa lease temporali, non richiede clock sincronizzati e non promette elezione se i timeout sono continuamente violati. Test [T-004–008 e T-021](testing-strategy.md) dovranno verificare transizioni e invarianti con simulatori e processi reali; l'argomento progettuale non sostituisce le evidenze.

## Confine dello schema persistente

I [formati canonici](canonical-formats.md) fissano kind log, comandi tipizzati, semantica transazioni e metadati snapshot/controllo. Decodificare un checkpoint commit valido non stabilisce che una transizione Raft runtime fosse legale; il consenso deve fornirne prova. Questa specifica non implementa Raft o adattatore noop/sessioni.

## Confine elettorale M05 implementato

[M05](milestone-m05.md) implementa New/Tick/Request/Response/Durable/Stop con millisecondi correnti espliciti, estrazioni iniettate e un effetto sincronizzato pendente. Recovery follower/termine/voto e confini termini copiati; conta sé solo dopo completamento esatto generazione/sequenza, quorum 2 poi sincronizza noop locale nuovo termine prima di pubblicare leader. Heartbeat vuoti riconoscono leadership/resettano timer e verificano prev senza eliminare log, avanzare commit o applicare. Effetti append non vuote/snapshot restano indisponibili, pur persistendo termini maggiori validi. Risposte tardive comunicano ancora termini maggiori; voti vecchi non eleggono un nuovo termine. Status.Pending marca campi provvisori, Failure li rende inutilizzabili come acknowledgment. Superati oracolo voto/termine/leader salvato su 20 seed e 100 round heartbeat sani. Replica log, nextIndex/matchIndex, commit/apply, sessioni e macchine stati snapshot restano fasi future; l’algoritmo generale sopra non è tutto implementato.

## Confine replica M06 implementato

[M06](milestone-m06.md) aggiunge EnableReplication(1..16) all’avvio, entry canoniche possedute, AppendLocal con ricevuta solo Sync locale, replace_suffix dal primo conflitto e acknowledgment esatto del prefisso durevole. Status.Progress copia valori next/match; Sent/Failed distinguono invio effettivo da offerte saltate e solo risposte correlate di generazione corrente aggiornano progresso. Hint conflitto limitati e match mai regressivo. Prefissi committed recuperati non sovrascrivibili. Algoritmo generale commit/apply sopra resta M07: leaderCommit trasportato ma non avanza checkpoint qui. Modalità/fixture M05 originali conservate; StartReplication abilita livello M06 completo.

## Commit e applicazione M07 implementati

[M07](milestone-m07.md) abilita confine maggioritario del termine corrente solo dopo prova replica correlata e persiste commit_index prima di cambiare CommitIndex o schedulare. Follower apprendono max(vecchio,min(leaderCommit,fine richiesta verificata)) solo dopo Sync log pertinente; traffico duplicato/obsoleto/prefisso rifiutato non lo avanza. Noop nuovo termine esistente permette commit indiretto vecchio prefisso. Core possiede LastApplied<=ScheduledApplyIndex<=CommitIndex, avanzando Scheduled solo su admission M02 e LastApplied solo su completamento future. ApplyEpoch=1 locale all’istanza; snapshot/epoch sostitutive restano M13. Modalità M05/M06 legacy disponibili, StartCommitted seleziona M07. Nessuna lettura pubblica/garanzia client.

## Autorizzazione completamento M08

[Service.Write](../../internal/raft/write.go) preserva algoritmi commit/applicazione M07. Lock ticket separa esclusione permanente prima di registrazione da possibili effetti disco. Quando registrazione può iniziare, cancellazione abbandona solo chiamante. Solo Sync locale mai completed: attendere risultato applicato esatto indice/termine, poi inviare controllo finale attraverso owner. Rifiuta errore, shutdown, non leader, termine cambiato o commit/apply insufficienti. Termine maggiore osservato prima dell’evento impedisce successo anche con comando committed/applicato. Scadenza conservazione risultato o supersessione produce incertezza. Nessuna tabella sessioni o nuovo formato persistente.

## Evidenze restart M09

[M09](milestone-m09.md) lascia transizioni core invariate. Immagine controllata a tre nodi ha termine 5, indice committed 1 e indice non committed 2 su ogni nodo. Ogni startup pubblica follower/termine 5 con LastApplied=ScheduledApplyIndex=CommitIndex=1 e LastIndex=2. Solo noop quorum successivo del termine corrente può committare/applicare indice 2. Kill reali dei processi testano inoltre recovery leader/follower, catch-up e conservazione prefisso confermato. Nessun termine, voto o commit dedotto da leadership precedente o lunghezza log grezza.

## Entry lettura termine corrente M10

[M10](milestone-m10.md) abilita proposte read(key) approvate, replica follower e replay sotto capacità startup esplicita. Ogni richiesta assegna indice fresco del termine corrente. Sync locale, match follower esattamente correlato, confine maggioranza termine corrente e checkpoint commit sincronizzato esistenti invariati. Stessa entry read può soddisfare condizione commit termine corrente; nessun round lease heartbeat/ReadIndex sostitutivo. ScheduledApplyIndex avanza solo su ammissione M02 effettiva, LastApplied su completamento e risultato lettura appartiene a tale indice/termine. Controllo owner esistente rifiuta termine maggiore osservato prima di autorizzazione risposta. Vecchie risposte non committano nuovo prefisso read; test legacy ack obsoleti invariati.

## M13 Fase C — 23 novembre 2026

La [Fase C](milestone-m13-runtime-recovery.md) è RUNTIME RECOVERY PHASE COMPLETED: RecoverCommitted ripristina S e applica soltanto S+1..C con M02, poi drena/chiude il worker isolato prima del nuovo applier a C. WAL e snapshot sono validati sotto LOCK continuo. L'avvio è esplicito via node.StartAPIWithSnapshotRecovery per v2; StartAPI/CLI ordinari mantengono il guard base zero. Nessun fallback con CURRENT/snapshot corrotti. KV/sessioni, hard state e A=L=C verificati con crash/restart follower/leader/tutti processi. C-01/OQ-05 confermato; sottoinsieme recovery C-02/OQ-01 verificato, C-02/C-03 complessivi CONDITIONALLY READY. T-015/AC-015/FR-015, T-016/AC-016/FR-016 e T-026/AC-026/NFR-006 completi aperti: compaction, cleanup e InstallSnapshot non implementati. M13 PARTIALLY COMPLETED, M14 non iniziata.
