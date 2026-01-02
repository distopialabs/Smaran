#!/bin/bash

curl -s https://eth-mainnet.g.alchemy.com/v2/3PBdxpi1SAyIg6TbJRf_glqpoWVkO3YT \
  -H 'content-type: application/json' \
  --data '{
    "jsonrpc":"2.0","id":1,"method":"eth_getBalance",
    "params":["0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "0x1208756"]
  }'