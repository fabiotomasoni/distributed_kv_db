# Fault injection locale deterministica

[Indice](../README.md) · [English](../../en/adr/ADR-008-fault-injection.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

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
