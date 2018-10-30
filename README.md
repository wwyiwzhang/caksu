# CAKSU

## Job Clean Controller
Job is a resource in Kubernetes which is normally used for one-time task. After job is created, it will spin up a pod to execute tasks. However, if we don't clean up the failed jobs in time, they will pile up and makes clean-up a bit difficult. It's generally recommended to delete the failed jobs after certain time. Here we set the time limit to 24 hours and it's currently hard-coded which will be changed in the future. 

## Install and Run 

### Local
To run the job cleaner locally, export `KUBECONFIG` to the location of k8s config on your local laptop. For example, `export KUBECONFIG=~/.kube/config`. Next step, execute `go build caksu/main.go`. Last, run `caksu/main`. 

### Cluster

