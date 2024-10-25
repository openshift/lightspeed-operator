# Release Tools

This directory contains tools to prepare new releases:
1. Update the bundle.
2. Download related image list from a Konflux snapshot.
3. Update the catalog from a Konflux snapshot.

## Bundle Update

When we update the bundle?
1. change in the CRD (change in api/v1alpha1/olsconfig_types.go)
2. change in any resources (deployment, role, service, etc.) in config/ directory

`update_bundle.sh` is the tool for updating bundle.
Normally we just need to specify a version for the bundle, using argument `-v`. For example this command updates the bundle with version `0.2.1`.
`./hack/update_bundle.sh -v 0.2.1`

We can also update the `.spec.relatedImages` field in the bundle by passing an image list JSON file using argument `-i`
`./hack/update_bundle.sh -v 0.2.1 -i related_images.json`

If related images is not specified, it keeps the `.spec.relatedImages` field in the ClusterServiceVersion file in the bundle.

We can also use `make bundle` to update the bundle.
- `BUNDLE_TAG=0.2.1  make bundle` generates a bunlde with version `0.2.1`
- `RELATED_IMAGES_FILE=related_images.json make bundle` generates a bundle with version `0.2.1` and images in the file `related_images.json`

Anyway, after building the bundle image from `bundle.Dockerfile` the `.spec.relatedImages` field in the file `/manifests/lightspeed-operator.clusterserviceversion.yaml` is set to the images in `related_images.json`.
Specifying the RELATED_IMAGES_FILE is for previewing the final bundle build.

## Image List Update

(Please login to Konflux before using this tool)

The image list file contains a JSON array listing at least 3 components' images: `lightspeed-service-api`, `lightspeed-console-plugin` and `lightspeed-operator`.

`snapshot_to_image_list.sh` is the tool to extract image list from a Konflux snapshot using its reference passed by argument `-s`.
For example, this command extract image references from the snapshot `ols-9xf2f` and save the list to the file `related_images.json`.
`/hack/snapshot_to_image_list.sh -s ols-9xf2f -o related_images.json`

If the `-o` argument is omitted, it will output to the stdout.

## Catalog Update

(Please login to Konflux before using this tool)

`snapshot_to_catalog.sh` is the tool to update catalog from Konflux snapshots.
We have to pass 3 arguments: `-s <snapshot-refs> -c <catalog-file> -n <channel-names>`
- `-s snapshot-refs` required, the snapshots' references to use, ordered by versions ascending, example: ols-cq8sl,ols-mdc8x"
- `-c catalog-file` optional, the catalog index file to update, default: lightspeed-catalog-4.16/index.yaml"
- `-n channel-names` the channel names to update, default: alpha"
For example, we generate the catalog to contain 2 bundles from the snapshots `ols-cq8sl,ols-mdc8x` in the `technical-preview` channel, saved to the index file `lightspeed-catalog-4.16/index.yaml`.
`./hack/snapshot_to_catalog.sh -s ols-cq8sl,ols-mdc8x  -n technical-preview -c lightspeed-catalog-4.16/index.yaml`

Attention that catalogs for OCP version 4.17 and later, the index file in JSON format is required. To generate the index in JSON format, we pass the `-m` argument, like this:
`./hack/snapshot_to_catalog.sh -s ols-cq8sl -n technical-preview -c lightspeed-catalog-4.16/index.yaml -m`

The JSON format index file works for all supported OCP version by Openshift Lightspeed. No need to refrain from using the `-m` arugment :)
