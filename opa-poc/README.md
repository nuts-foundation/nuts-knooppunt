```shell
curl -L -o opa https://openpolicyagent.org/downloads/latest/opa_darwin_arm64
chmod +x opa
```

Running:

```shell
./opa eval -i input.json -d example.rego "data.example.violation[x]"
```