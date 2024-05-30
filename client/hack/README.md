# Scripts User Guide

This README documents:

* What update-crd.sh and update-generated-code.sh do
* When and how to use them

## update-generated-code.sh

This is the script to update clientset/informers/listers and API deepcopy code using [code-generator](https://github.com/kubernetes/code-generator).

Make sure to run this script after making changes to /client/apis/volumesnapshot/v1/types.go.

### Pre-requisites for running update-generated-code.sh:

* Set `GOPATH`
    ```bash
    export GOPATH=~/go
    ```

* Ensure external-snapshot-metadata repository is at `~/go/src/github.com/kubernetes-csi/external-snapshot-metadata`

* Clone code-generator 
    ```bash
    cd ~/go/src/k8s.io
    git clone https://github.com/kubernetes/code-generator.git 
    ```
* Checkout latest release version
    ```bash
    git checkout v0.30.0
    ```

* Ensure the file `kube_codegen.sh` exists

    ```bash
    ls ${GOPATH}/src/k8s.io/code-generator/kube_codegen.sh
    ```
  
Update generated client code in external-snapshot-metadata
    
```bash
    cd ~/go/src/github.com/kubernetes-csi/external-snapshot-metadata/client
    ./hack/update-generated-code.sh
``` 

Once you run the script, the code will be generated for snapshotmetadataservice:v1alpha1, and you will get an output as follows:
    
```bash
Generating deepcopy code for 1 targets
Generating client code for 1 targets
Generating lister code for 1 targets
Generating informer code for 1 targets
```

## update-crd.sh

This is the script to update CRD yaml files under /client/config/crd/ based on types.go file.

Make sure to run this script after making changes to /client/apis.

Follow these steps to update the CRD:

* Run ./hack/update-crd.sh from client directory, new yaml files should have been created under ./config/crd/

* Add api-approved.kubernetes.io annotation value in all yaml files in the metadata section with the PR where the API is approved by the API reviewers. Refer to https://github.com/kubernetes/enhancements/pull/1111 for details about this annotation.


## Test suite 

// TODO: Add CEL tests