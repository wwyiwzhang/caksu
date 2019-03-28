package main

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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
	defaultProcessingDelay = time.Duration(50000000000)
	defaultTimeLimit       = 24.0
	defaultNamespaceFile   = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	defaultNamespaceName   = "default"
)

type JobCleanController struct {
	client          kubernetes.Interface
	deleteJob       func(namespace string, key string) error
	jobLister       lister_v1.JobLister
	informer        cache.Controller
	namespace       string
	passedTimeLimit func(t time.Time) bool
	queue           workqueue.RateLimitingInterface
}

var log *logrus.Logger

func main() {

	// load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)

	if err != nil {
		log.Fatalf("failed to load kubeconfig, %v", err)
	}

	// create k8s client
	client, err := kubernetes.NewForConfig(config)

	if err != nil {
		log.Fatalf("failed to create kubernetes client, %v", err)
	}

	stopChan := make(chan struct{})
	defer close(stopChan)

	newJobCleanController(client).Run(stopChan)
}

func newJobCleanController(client kubernetes.Interface) *JobCleanController {

	timeLimit := getTimeLimit()
	jc := &JobCleanController{
		client:    client,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		namespace: getNamespace(),
		deleteJob: func(namespace string, key string) error {
			return client.BatchV1().Jobs(namespace).Delete(key, &meta_v1.DeleteOptions{})
		},
		passedTimeLimit: func(t time.Time) bool {
			return time.Since(t).Hours() > timeLimit
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
				job := obj.(*v1.Job)
				jc.queue.Add(job.GetName())
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

	log.Info("starting Job Clean Controller")
	go jc.informer.Run(stopChan)

	for {
		jc.processNext()
	}
}

func (jc *JobCleanController) processNext() bool {
	key, quit := jc.queue.Get()
	if quit {
		return false
	}
	defer jc.queue.Done(key)
	err := jc.process(key.(string))
	if err != nil {
		log.Warningf("skipped job: %s, %v", key, err)
		return false
	}
	return true
}

func (jc *JobCleanController) process(key string) error {
	// determine the job duration between now and start time
	// if the job duration > time limit and has no active pods then delete the job
	job, err := jc.jobLister.Jobs(jc.namespace).Get(key)
	if err != nil {
		log.Errorf("failed to retrieve job: %s, %v", key, err)
		return err
	}

	log.Infof("retrieved job: %s", key)
	jobStatus := job.Status
	if jobStatus.StartTime == nil {
		jc.queue.AddAfter(key, defaultProcessingDelay)
		return nil
	}
	start, err := parseTime(jobStatus.StartTime.String())
	if err != nil {
		log.Errorf(
			"failed to parse start time: %s for job: %s, add back to the queue, %v",
			jobStatus.StartTime.String(),
			key,
			err,
		)
		jc.queue.AddAfter(key, defaultProcessingDelay)
		return err
	}
	if jobStatus.Active == 0 && jc.passedTimeLimit(start) {
		err := jc.deleteJob(jc.namespace, key)
		if err != nil {
			log.Errorf("failed to delete job: %s, add back to the queue, %v", key, err)
			jc.queue.AddAfter(key, defaultProcessingDelay)
			return err
		}
		log.Infof("deleted job: %s in namespace: %s", key, jc.namespace)
		return nil
	}
	jc.queue.AddAfter(key, defaultProcessingDelay)
	return nil
}

func getNamespace() string {
	namespace := os.Getenv("POD_NAMESPACE")
	if len(namespace) == 0 {
		data, err := ioutil.ReadFile(defaultNamespaceFile)
		if err != nil {
			log.Warningf(
				"failed to load namespace from: %s, use default namespace instead, %v",
				defaultNamespaceFile,
				err,
			)
			namespace = defaultNamespaceName
		} else {
			namespace = strings.TrimSpace(string(data))
		}
	}
	return namespace
}

func getTimeLimit() float64 {
	timelimitStr := os.Getenv("JOB_TIME_LIMIT_IN_HOURS")
	if len(timelimitStr) == 0 {
		return defaultTimeLimit
	}
	timelimit, err := strconv.ParseFloat(timelimitStr, 64)
	if err != nil {
		log.Errorf(
			"failed to convert time limit: '%s' to float64, use default time limit instead",
			timelimitStr,
		)
		return defaultTimeLimit
	}
	log.Infof("time limit to delete failed jobs: %f (hours)", timelimit)
	return timelimit
}

func parseTime(timeStr string) (time.Time, error) {
	var format string

	switch {
	case strings.Contains(timeStr, "CST"):
		format = "2006-01-02 15:04:05 +0800 CST"
	case strings.Contains(timeStr, "UTC"):
		format = "2006-01-02 15:04:05 +0000 UTC"
	case strings.Contains(timeStr, "PDT"):
		format = "2006-01-02 15:04:05 -0700 PDT"
	default:
		format = "2006-01-02T15:04:05Z07:00"
	}
	parsed, err := time.Parse(format, timeStr)
	return parsed, err
}

func init() {
	log = logrus.New()
	log.SetReportCaller(true)
}
