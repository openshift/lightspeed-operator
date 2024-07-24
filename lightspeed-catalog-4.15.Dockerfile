FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.15

# Configure the entrypoint and command
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

# Copy licenses required by Red Hat certification policy
ADD licenses/ /licenses/
# Copy declarative config root into image at /configs and pre-populate serve cache
ADD lightspeed-catalog-4.15 /configs
RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# Set DC-specific label for the location of the DC root directory
# in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs
