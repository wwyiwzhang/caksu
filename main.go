package main

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	v1 "k8s.io/api/batch/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	lister_v1 "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

const (
	defaultTimeLimit     string = "24"
	defaultNamespaceFile string = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	defaultNamespaceName string = "default"
)

type JobCleanController struct {
	client    kubernetes.Interface
	deleteJob func(namespace string, key string) error
	jobLister lister_v1.JobLister
	informer  cache.Controller
	namespace string
	queue     workqueue.RateLimitingInterface
}

func main() {

	// load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)

	if err != nil {
		glog.Fatalf("failed to load kubeconfig, %v", err)
	}

	// create k8s client
	client, err := kubernetes.NewForConfig(config)

	if err != nil {
		glog.Fatalf("failed to create kubernetes client, %v", err)
	}

	stopChan := make(chan struct{})
	defer close(stopChan)

	newJobCleanController(client).Run(stopChan)
}

func newJobCleanController(client kubernetes.Interface) *JobCleanController {
	jc := &JobCleanController{
		client:    client,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		namespace: getNamespace(),
		deleteJob: func(namespace string, key string) error {
			return client.BatchV1().Jobs(namespace).Delete(key, &meta_v1.DeleteOptions{})
		},
	}

	//TODO: resource version
	indexer, informer := cache.NewIndexerInformer(
		&cache.ListWatch{
			ListFunc: func(lo meta_v1.ListOptions) (runtime.Object, error) {
				return client.BatchV1().Jobs(jc.namespace).List(lo)
			},
			WatchFunc: func(lo meta_v1.ListOptions) (watch.Interface, error) {
				return client.BatchV1().Jobs(jc.namespace).Watch(lo)
			},
		},
		&v1.Job{},
		10*time.Second,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
					jc.queue.Add(key)
				}
			},
		},
		cache.Indexers{},
	)

	jc.informer = informer
	jc.jobLister = lister_v1.NewJobLister(indexer)

	return jc
}

func (jc *JobCleanController) Run(stopChan chan struct{}) {
	defer jc.queue.ShutDown()

	glog.Info("starting Job Clean Controller")
	go jc.informer.Run(stopChan)

	for {
		jc.processNext()
	}

	<-stopChan
	glog.Info("shut down Job Clean Controller")
}

func (jc *JobCleanController) processNext() bool {
	key, quit := jc.queue.Get()
	if quit {
		return false
	}
	defer jc.queue.Done(key)

	jobName := strings.Split(key.(string), "/")[1]
	err := jc.process(jobName)

	if err != nil {
		glog.Warningf("skipped job: %s", jobName)
	}
	return true
}

func (jc *JobCleanController) process(key string) error {
	// determine the job duration between now and start time
	// if the job duration > time limit and has no active pods then delete the job

	timelimitStr := os.Getenv("JOB_TIME_LIMIT_IN_HOURS")
	if len(timelimitStr) == 0 {
		timelimitStr = defaultTimeLimit
	}
	timelimit, err := strconv.ParseFloat(timelimitStr, 64)
	if err != nil {
		glog.Errorf("failed to convert time limit: '%s' to float64", timelimitStr)
		return err
	}

	job, err := jc.jobLister.Jobs(jc.namespace).Get(key)
	if err != nil {
		glog.Errorf("failed to retrieve job: %s, %v", key, err)
		return err
	}

	glog.Infof("retrieved job: %s", key)
	jobStatus := job.Status
	namespacedKey := strings.Join([]string{jc.namespace, key}, "/")
	if jobStatus.StartTime == nil {
		// job has no start time
		return nil
	}
	start, err := parseTime(jobStatus.StartTime.String())
	if err != nil {
		glog.Errorf("failed to parse start time: %s for job: %s, add back to the queue, %v",
			jobStatus.StartTime.String(), key, err)
		jc.queue.AddAfter(namespacedKey, time.Duration(50000000000))
		return err
	}
	if jobStatus.Active == 0 && sinceNow(start) > timelimit {
		err := jc.deleteJob(jc.namespace, key)
		if err != nil {
			glog.Errorf("failed to delete job: %s, add back to the queue", key)
			jc.queue.AddAfter(namespacedKey, time.Duration(50000000000))
			return err
		}
		glog.Infof("deleted job: %s in namespace: %s", key, jc.namespace)
	}
	return nil
}

func getNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if len(namespace) == 0 {
		data, err := ioutil.ReadFile(defaultNamespaceFile)
		if err != nil {
			glog.Warningf("failed to load namespace from: %s, use default namespace instead, %v",
				defaultNamespaceFile, err)
			namespace = defaultNamespaceName
		} else {
			namespace = strings.TrimSpace(string(data))
		}
	}
	return namespace
}

func sinceNow(startTime time.Time) float64 {
	now, err := time.Parse("2006-01-02 15:04:05", time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		glog.Errorf("failed to parse current timestamp in format '%s', %v",
			"2006-01-02 15:04:05", err)
		return 0.0
	}
	return now.Sub(startTime).Hours()
}

func parseTime(timeStr string) (time.Time, error) {
	var format string

	switch {
	case strings.Contains(timeStr, "CST"):
		format = "2006-01-02 15:04:05 +0800 CST"
	case strings.Contains(timeStr, "UTC"):
		format = "2006-01-02 15:04:05 +0000 UTC"
	default:
		format = "2006-01-02T15:04:05Z07:00"
	}
	parsed, err := time.Parse(format, timeStr)
	return parsed, err
}
