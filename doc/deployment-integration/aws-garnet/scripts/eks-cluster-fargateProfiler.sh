cat << EOF > ./yaml/eks-cluster-3az.yaml
# A simple example of ClusterConfig object:
---
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig

metadata:
  name: ${ekscluster_name}
  region: ${AWS_REGION}
  version: "${eks_version}"

vpc:
  id: ${vpc_ID}
  subnets:
    private:
      PrivateSubnet01:
        az: ${AWS_REGION}a
        id: ${PrivateSubnet01}
      PrivateSubnet02:
        az: ${AWS_REGION}b
        id: ${PrivateSubnet02}
      PrivateSubnet03:
        az: ${AWS_REGION}c
        id: ${PrivateSubnet03}

secretsEncryption:
  keyARN: ${MASTER_ARN}

fargateProfiles:
  - name: eks-fp
    selectors:
      - namespace: ips
      - namespace: kube-system
    subnets:
      - ${PrivateSubnet01}
      - ${PrivateSubnet02}
      - ${PrivateSubnet03}

cloudWatch:
  clusterLogging:
    enableTypes:
      ["api", "audit", "authenticator", "controllerManager", "scheduler"]

EOF