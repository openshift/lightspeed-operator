FROM brew.registry.redhat.io/rh-osbs/openshift-ose-operator-registry-rhel9:v4.19

# Configure the entrypoint and command
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

# Copy licenses required by Red Hat certification policy
ADD LICENSE /licenses/
# Copy declarative config root into image at /configs/lightspeed-operator and pre-populate serve cache
ADD lightspeed-catalog-4.19 /configs/lightspeed-operator
RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# Set DC-specific label for the location of the DC root directory
# in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs
LABEL cpe="cpe:/a:redhat:openshift_lightspeed:1::el9"
