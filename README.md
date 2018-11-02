# CAKSU

## Job Clean Controller
Job is a resource in Kubernetes which is normally used for one-time task. After job is created, it will spin up a pod to execute the task. However, if we don't clean up the failed jobs in time, they will pile up and makes clean-up a bit difficult later. 
It's generally recommended to delete the failed jobs after certain time. Here we can set the time limit by exporting `JOB_TIME_LIMIT_IN_HOURS` as an environment variable. If `JOB_TIME_LIMIT_IN_HOURS` is not set, the job clean controller will use the default time limit which is 24 hours.   
## Install and Run
### Local
To run the job cleaner locally, export `KUBECONFIG` to the location of k8s config on your local laptop. For example, `export KUBECONFIG=~/.kube/config`. Next step, execute `go build caksu/main.go`. Last, run `caksu/main`. 
### Cluster
To run the job cleaner in a cluster, create a deployment following `job-clean-controller.yaml` in the `example` folder.
## References
