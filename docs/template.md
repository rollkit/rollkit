# Protocol/Component Name

## Abstract

Provide a concise description of the purpose of the component for which the
specification is written, along with its contribution to the rollkit or
other relevant parts of the system. Make sure to include proper references to
the relevant sections.

## Protocol/Component Description

Offer a comprehensive explanation of the protocol, covering aspects such as data
flow, communication mechanisms, and any other details necessary for
understanding the inner workings of this component.

## Message Structure/Communication Format

If this particular component is expected to communicate over the network,
outline the structure of the message protocol, including details such as field
interpretation, message format, and any other relevant information.

## Assumptions and Considerations

If there are any assumptions required for the component's correct operation,
performance, security, or other expected features, outline them here.
Additionally, provide any relevant considerations related to security or other
concerns.

## Implementation

Include a link to the location where the implementation of this protocol can be
found. Note that specific implementation details should be documented in the
rollkit repository rather than in the specification document.

## References

List any references used or cited in the document.

## General Tips

### How to use a mermaid diagram that you can display in a markdown

```mermaid

sequenceDiagram
    title Example
    participant A
    participant B
    A->>B: Example
    B->>A: Example

 ```

 ```mermaid

graph LR
    A[Example] --> B[Example]
    B --> C[Example]
    C --> A

 ```

 ```mermaid

gantt
    title Example
    dateFormat  YYYY-MM-DD
    section Example
    A :done,    des1, 2014-01-06,2014-01-08
    B :done,    des2, 2014-01-06,2014-01-08
    C :done,    des3, 2014-01-06,2014-01-08

 ```

### Grammar and spelling check

The recommendation is to use your favorite spellchecker extension in your IDE like [grammarly], to make sure that the document is free of spelling and grammar errors.

### Use of links

If you want to use links use proper syntax. This goes for both internal and external links like [documentation] or [external links]

At the bottom of the document in [Reference](#references), you can add the following footnotes that will be visible in the markdown document:

[1] [Grammarly][grammarly]

[2] [Documentation][documentation]

[3] [external links][external links]

Then at the bottom add the actual links that will not be visible in the markdown document:

[grammarly]: https://www.grammarly.com/
[documentation]: ../README.md
[external links]: https://github.com/celestiaorg/go-header

### Use of tables

If you are describing variables, components or other things in a structured list that can be described in a table use the following syntax:

| Name | Type | Description |
| ---- | ---- | ----------- |
| `name` | `type` | Description |
