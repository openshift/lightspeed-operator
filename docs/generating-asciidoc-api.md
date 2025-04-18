# Generating AsciiDoc API reference

## One-time setup docsgen repo

1. `git clone https://github.com/jboxman-rh/openshift-apidocs-gen`
2. `cd openshift-apidocs-gen`
3. `npm install`
4. PATH=<path_to_>/openshift-apidocs-gen:$PATH

## Run it

```bash
# If you haven't already, start any k8s cluster - kind, CRC
kind create cluster

# Make sure the CRD has all the desired doc within
make generate

make install
hack/asciidoc-gen.sh
```
