#!/bin/bash

curl -s https://eth-mainnet.g.alchemy.com/v2/3PBdxpi1SAyIg6TbJRf_glqpoWVkO3YT \
  -H 'content-type: application/json' \
  --data '{
    "jsonrpc":"2.0","id":1,"method":"eth_getProof",
    "params":["0xC479E4B11885B56Bfc03E1eba4F232484C35Ad51", [], "0x12086df"]
  }'