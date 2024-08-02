#!/usr/bin/env sh

echo "Waiting for the faucet to start..."

# wait until port 1095 is available
while ! nc -z localhost 1095; do
  sleep 0.1
done

# get the address of the account using cel-key which is a cosmos-sdk binary
ADDRESS=$(cel-key list --keyring-dir /home/apollo/.apollo/light-node/keys/ --output json | jq .[0].address| sed 's/^.\(.*\).$/\1/')

echo "Funding account $ADDRESS"

# fund the account with some tokens
RESULT=$(curl -sS http://localhost:1095/fund/$ADDRESS)

# check that the account was funded if the result contains "Successfully"
if echo "$RESULT" | grep -q "Successfully"; then
  echo "Account funded successfully"
else
  echo "Failed to fund account"
  exit 1
fi