# Legacy Support

Now lots of apps are still running on K8s clusters version v1.14, v1.15, while oam-k8s-runtime requires the minimum
K8s version to be v1.16.

Currently, the main block is OAM runtime use CRD v1, while these old K8s versions don't support CRD v1.
So we generate v1beta1 CRD here for convenience. But we have no guarantee that oam-runtime will support the
legacy k8s versions. 

