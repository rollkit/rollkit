# Store

## Abstract

The Store is responsible for storing and retrieving blocks, commits, and state. It provides an interface for other components to interact with the stored data, ensuring data consistency and integrity.

## Protocol/Component Description

The Store interface defines methods for saving and loading blocks, commits, and state. It also provides methods for setting and retrieving the height of the highest block in the store. The DefaultStore struct is an implementation of the Store interface, using a transactional datastore for underlying storage.

## Message Structure/Communication Format

The Store does not communicate over the network, so there is no message structure or communication format.

## Assumptions and Considerations

The Store assumes that the underlying datastore is reliable and provides atomicity for transactions. It also assumes that the data passed to it for storage is valid and correctly formatted.

## Implementation

The implementation of the Store interface can be found in the `store/store.go` file in the Rollkit repository.

## References

- `store/store.go`: Contains the implementation of the Store interface.
- `store/types.go`: Defines the Store interface and related types.
- `block/manager.go`: Uses the Store interface for storing and retrieving blocks and state.
