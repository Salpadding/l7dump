find . -type f | grep '.go$' | xargs goimports -w 
