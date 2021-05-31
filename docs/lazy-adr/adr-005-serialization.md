# ADR 005: Serialization

## Changelog

- 2021-05-31: Created

## Context

All the basic data types needs to be efficiently serialized into binary format before saving in KV store or sending to network.

## Alternative Approaches

There are countless alternatives to `protobuf`, including `flatbuffers`, `avro`, `ASN.1`, `RLP`.

## Decision

`protobuf` is used for data serialization both for storing and network communication. 
`protobuf` is used widely in entire Cosmos ecosystem, and we would need to use it anyways.

## Status

{Accepted}

## Consequences

### Positive
 * well known serialization method
 * language independent

### Negative
 * there are known issues with `protobuf`

### Neutral
 * it's de-facto standard in Cosmos ecosystem

