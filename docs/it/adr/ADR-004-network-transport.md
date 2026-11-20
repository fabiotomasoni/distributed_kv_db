# Trasporto HTTP/JSON locale

[Indice](../README.md) · [English](../../en/adr/ADR-004-network-transport.md)

**Stato: progetto pre-implementativo, revisione 0.2 — 18 novembre 2026. Nessuna funzionalità o misura è verificata.**

## Identificatore e stato

**ADR-004 — Trasporto HTTP/JSON locale**. Stato **Proposed**, 18 novembre 2026. Decisione di progetto da confermare ai gate indicati, non evidenza implementativa.

## Contesto

RPC devono essere ispezionabili e funzionare offline senza generatori.

## Problema

Definire framing, errori e backpressure senza sovrainfrastruttura.

## Alternative valutate

HTTP/JSON standard; TCP binario custom; gRPC/protobuf.

## Decisione

HTTP/1.1 loopback, JSON rigoroso, schema v1 e una RPC attiva per peer.

## Motivazione

Facilita diagnosi e limita dipendenze, adeguato al carico ridotto.

## Conseguenze

Limiti body/entry, indici decimal-string, deadline e correlazione espliciti.

## Svantaggi e limiti

Base64, parsing e assenza di TLS non soddisfano uso pubblico.

## Requisiti collegati

FR-003, FR-009, NFR-005, NFR-010. Vedere [matrice](../traceability.md).

## Validazione e questioni aperte

M04/M08: fixture interoperabili e OpenAPI client prima di stabilità API. Se i gate falliscono, aggiornare o sostituire questo ADR in entrambe le lingue; conservare ID e storia della decisione.

## Riferimenti

[Specifica correlata](../network-protocol.md); [fonti REF-04/12](../references.md); [roadmap](../implementation-roadmap.md).

## Rivalutazione audit 0.2

HTTP/JSON resta adeguato. Cap client corretto da 32 a 48 KiB per CAS con due valori massimi; cap peer invariato. Decode rigoroso richiede scansione duplicate key, UTF-8, nomi esatti e fine documento, oltre al decode standard. T-003/T-009 devono esercitare i limiti. Nessun nuovo pacchetto esterno.

## Evidenze implementazione M04 — 20 novembre 2026

[M04](../milestone-m04.md) implementa la decisione tramite [configurazione/hash](../configuration.md) immutabili, fixture protocollo rigorose e HTTP loopback limitato con libreria standard. Identità storage/LOCK precedono bind di produzione; shutdown drena prima del rilascio. Nessun DNS/proxy/redirect/compressione o dipendenza esterna. Una RPC in uscita per peer, socket/deadline limitati. Decisioni callback e futuri timing/replica Raft restano fuori M04. Motivazioni originali e limiti loopback fidato invariati.

## Evidenze implementazione M08 — 21 novembre 2026

[M08](../milestone-m08.md) aggiunge HTTP PUT/DELETE e [sottoinsieme OpenAPI](../../openapi/m08.json) verificato offline con fixture HTTP comuni. Porte peer/client configurate distinte richiedono due server standard library che condividono limitatore socket accettati M04; nessun protocollo RPC indipendente. Cap JSON client resta 48 KiB. Solo hint membri configurati, niente forwarding/redirect/proxy. C-01 soddisfatto per sottoinsieme M08; GET/CAS e stabilità API finale restano gate successivi.
