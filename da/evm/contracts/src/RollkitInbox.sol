pragma solidity ^0.8.19;

contract RollkitInbox {
    mapping(bytes32 => bytes) public namespaceToLatestBlock;

    function SubmitBlock(bytes32 namespace, bytes calldata data) public {
        namespaceToLatestBlock[namespace] = data;
    }

    function GetLatestBlock(
        bytes32 namespace
    ) public view returns (bytes memory) {
        return namespaceToLatestBlock[namespace];
    }
}
