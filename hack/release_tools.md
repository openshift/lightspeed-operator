# Release Tools

This directory contains tools to prepare new releases:
1. Update the bundle.
2. Download related image list from a Konflux snapshot.
3. Update the catalog from a Konflux snapshot.

## Bundle Update

When we update the bundle?
1. change in the CRD (change in api/v1alpha1/olsconfig_types.go)
2. change in any resources (deployment, role, service, etc.) in config/ directory

`update_bundle.sh` updates the bundle. It requires a bundle variant (`v1` or `v2`) before its options; the bundle version passed with `-v` must use the variant's major version. For example, this command updates the classic bundle with version `1.2.1`.
`./hack/update_bundle.sh v1 -v 1.2.1`

Pass the required image list JSON file with `-i` to update the variant-filtered `.spec.relatedImages` field in the bundle:
`./hack/update_bundle.sh v1 -v 1.2.1 -i related_images.json`

We can also use `make bundle` to update the bundle.
- `BUNDLE_VARIANT=v1 BUNDLE_TAG=1.2.1 make bundle` generates a classic bundle with version `1.2.1` using `related_images.json`.
- `BUNDLE_VARIANT=v2 BUNDLE_TAG=2.0.0 make bundle` generates the agentic bundle with version `2.0.0` using `related_images.json`.

After building the bundle image from `bundle.Dockerfile`, the `.spec.relatedImages` field in `/manifests/lightspeed-operator.clusterserviceversion.yaml` is set to the variant-filtered images in `related_images.json`.

## Image List Update

(Please login to Konflux before using this tool)

The image list file contains a JSON array listing at least 3 components' images: `lightspeed-service-api`, `lightspeed-console-plugin` and `lightspeed-operator`.

`snapshot_to_image_list.sh` is the tool to extract image list from a Konflux snapshot using its reference passed by argument `-s`.
For example, this command extract image references from the snapshot `ols-9xf2f` and ols-bundle snapshot `ols-bundle-2dhtr` and save the list to the file `related_images.json`.
`/hack/snapshot_to_image_list.sh -s ols-9xf2f -b ols-bundle-2dhtr -o related_images.json`

If the `-o` argument is omitted, it will output to the stdout.

## Catalog Update

(Please login to Konflux before using this tool)

`snapshot_to_catalog.sh` is the tool to update catalog from Konflux snapshots.
We have to pass 4 arguments: `-s <snapshot-ref> -b <bundle-snapshot-ref> -c <catalog-file> -n <channel-names>`
- `-s snapshot-ref` required, the snapshots' references to use"
- `-b bundle-snapshot-ref` required, the bundle snapshots' references to use"
- `-c catalog-file` optional, the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml"
- `-n channel-names` the channel names to update, default: alpha"
For example, we generate the catalog from the ols snapshot `ols-cq8sl` and ols-bunlde snapshot `ols-bundle-r578d` in the `technical-preview` channel, saved to the index file `lightspeed-catalog-4.16/index.yaml`.
`./hack/snapshot_to_catalog.sh -s ols-cq8sl -b ols-bundle-r578d  -n technical-preview -c lightspeed-catalog-4.16/index.yaml`

Attention that catalogs for OCP version 4.17 and later, the index file in JSON format is required. To generate the index in JSON format, we pass the `-m` argument, like this:
`./hack/snapshot_to_catalog.sh -s ols-cq8sl  -b ols-bundle-r578d -n technical-preview -c lightspeed-catalog-4.16/index.yaml -m`

The JSON format index file works for all supported OCP version by Openshift Lightspeed. No need to refrain from using the `-m` arugment :)
