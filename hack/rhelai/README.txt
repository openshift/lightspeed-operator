= RHELAI with TLS

== Replace the IP address

The following assumes the public IP of the RHELAI EC2 instance is 18.191.144.20. Find out the public IP of the RHELAI EC2 instance you're working with and replace 18.191.144.20 in openssl.cnf and nginx.conf.

== Generate the signing key and the certificate

You're going to need a signing key and a certificate for the TLS endpoint. Generate both with the following command on your laptop:

``` bash
$ openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout certs/rhelai.key -out certs/rhelai.crt -config openssl.cnf
```
They are going to end up in the certs directory:

$ ls -l certs/
total 8
-rw-r--r--. 1 username username 1403 Jun 17 14:08 rhelai.crt
-rw-------. 1 username username 1704 Jun 17 14:08 rhelai.key
$

== Copy the certs and NGINX config over to the RHELAI instance

```bash
$ scp -i your_aws_keys.pem -r certs nginx.conf cloud-user@18.191.144.20:~
```

== Run nginx container on the RHELAI instance

Nginx plays the role of a reverse proxy, accepting TLS connections from the OLS instance on the cluster and forwarding traffic over HTTP to the vLLM server. Its worth repeating, this runs on the RHELAI instance:

```bash
$ podman run -it --rm --network=host \
  --name my-nginx \
  -p 8443:8443 \
  -v $PWD/nginx.conf:/etc/nginx/nginx.conf:ro,Z \
  -v $PWD/certs:/etc/nginx/certs:ro,Z \
  docker.io/library/nginx:stable
```

Nginx will log to the terminal, so it's going to be easy to see if any issues arise.

== Configure the OLS to talk to the TLS endpoint on the RHELAI instance

=== Create a ConfigMap with the nginx certificate in it:

```bash
$ oc project openshift-lightspeed
Now using project "openshift-lightspeed" on server "https://api.crc.testing:6443".
$ oc create configmap rhelai-ca --from-file=certs/rhelai.crt
configmap/rhelai-ca created
```

Create an OLSConfig instance from the following YAML, replacing the IP address with the one for your RHELAI instance. Note spec.ols.additionalCAConfigMapRef referring to the ConfigMap we just created.

```yaml
---
apiVersion: v1
data:
  apitoken: Zm9vYmFyCg==
kind: Secret
metadata:
  name: openai-api-keys
  namespace: openshift-lightspeed
type: Opaque
---
apiVersion: ols.openshift.io/v1alpha1
kind: OLSConfig
metadata:
  name: cluster
  namespace: openshift-lightspeed
spec:
  llm:
    providers:
    - credentialsSecretRef:
        name: openai-api-keys
      models:
      - name: /var/home/cloud-user/.cache/instructlab/models/granite-7b-redhat-lab
        contextWindowSize: 4096
        parameters:
          maxTokensForResponse: 2048
      name: rhelai_vllm
      type: rhelai_vllm
      url: https://18.191.144.20:8443/v1
  ols:
    defaultProvider: rhelai_vllm
    defaultModel: /var/home/cloud-user/.cache/instructlab/models/granite-7b-redhat-lab
    logLevel: DEBUG
    additionalCAConfigMapRef:
      name: rhelai-ca
```
