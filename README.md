# score-k8s



## FAQ


### What's the implementation of placeholders and interpolation?

A resource can return outputs. A map of key-values. There is no difference in format between plaintext and secrets, however any secrets are output using a MAGIC (🪄) wrapping mechanism: `"LQNYUJB" encoded(<secret-name>/<key-name>) "BJUYNQL"`. When the final manifests are generated, these are converted into secret references or failures are generated.

The source of the secrets names and keys are from either pre-existing secrets that are assumed to exist, secret manifests generated by resources, or in some cases secrets that are inserted to the destination cluster by the provisioners.

### How would a volume provisioner work?

Volumes and volume mounts are interesting... similar to flyio, the volume must be replicated per pod replica. So it can't just be provisioned as a resource the way other things can.

We need to be able to reason about ephemeral vs persistent volumes.

score-humanitec converts volumes not into a volume but into the spec of a volume and assembles these at deployment time.

So we could do something similar... we convert it into the volume spec and link.



