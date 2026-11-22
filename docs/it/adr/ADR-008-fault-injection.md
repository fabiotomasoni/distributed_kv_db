# Fault injection locale deterministica

[Indice](../README.md) · [English](../../en/adr/ADR-008-fault-injection.md)

**22 novembre 2026 — profilo M12 implementato e verificato; sezioni progettuali storiche precisate dal contratto M12 in fondo.**

## Identificatore e stato

**ADR-008 — Fault injection locale deterministica**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

Guasti rari non sono riprodotti affidabilmente da stress casuale.

## Problema

Esplorare casi limite mantenendo test brevi sul Mac.

## Alternative valutate

Sleep casuali; proxy/VM esterne; simulatori/adattatori in-process e harness locale.

## Decisione

Clock e rete virtuali, storage con cutpoint, seed+traccia; processi reali per integrazione.

## Motivazione

Consente replay e riduzione controesempi senza infrastruttura o carico massivo.

## Conseguenze

Fault disabilitati normalmente, manifest obbligatorio e cleanup per run.

## Svantaggi e limiti

Il modello può essere errato; seed non determina scheduling OS.

## Requisiti collegati

FR-017, FR-020, NFR-008. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M12/M14: replay identico in simulazione e casi reali complementari. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../fault-injection.md); [fonti REF-01/07/09](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

Separati ProcessCrash e SystemCrashModel: write non sincronizzate possono sopravvivere, e un kill non elimina la cache del kernel. Cutpoint devono coprire frame interi/parziali e rename visibili. Fondamenta fake a M02–M05, completamento framework a M12. Nessun simulatore è prova di power loss reale.

## Evidenze implementative M02

19 novembre 2026: [M02](../milestone-m02.md) aggiunge piccoli clock manuale e fake transport/storage solo nei test e una dipendenza Machine bloccabile. Ritardo controllato, rifiuto, consegna duplicata e cleanup worker verificati senza I/O del sistema o sleep casuali. Nessun replay seed/trace completo, cutpoint storage o modello crash implementato; AC-028 resta aperto a M12 e questo ADR resta Proposed.

## Evidenze implementazione M07

[M07](../milestone-m07.md) conserva decisione per prova quorum termine corrente, commit_index sincronizzato, replay stato volatile nuovo tramite M02 invariato e cursori completamento espliciti. Superati oracoli committed/risultati indipendenti, backlog bloccato, tagli checkpoint e test loopback complementari. Nessun checkpoint aggiuntivo, byte persistente nuovo o goroutine per entry. Evidenze C-03 M07 non chiudono gate successivi snapshot/sessioni/read/checker o garanzie guasti fisici.

## Decisione M12 accettata ed evidenze

22 novembre 2026: decisione Accepted nel profilo limitato [M12](../milestone-m12.md). Core invariato, scheduler esplicito, draw tape versionato e manifest completo, replay senza nuove scelte e prima divergenza, wrapper WAL con modelli ProcessCrash/SystemCrashModel separati e M02 reale. I/O storage sincrono come il driver corrente; nessuna simulazione snapshot/generazione M13. Il binario separato impedisce fault nel server ordinario. Log/metriche operativi limitati non sono oracoli; trace invarianti non campionate. Riduzione locale di azioni preserva solo il predicato/precondizioni forniti, non sostituisce checker M14. Regressioni e casi reali complementari passano; nessuna dimostrazione OS/alimentazione.
