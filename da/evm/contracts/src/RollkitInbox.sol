// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

contract RollkitInbox {
    mapping(bytes32 => mapping(uint256 => bytes))
        public namespaceToDaHeightBlock;

    function SubmitBlock(bytes32 namespace, bytes calldata data) public {
        namespaceToDaHeightBlock[namespace][block.number] = data;
    }

    function RetrieveBlock(
        bytes32 namespace,
        uint64 daHeight
    ) public view returns (bytes memory) {
        return namespaceToDaHeightBlock[namespace][uint256(daHeight)];
    }
}
