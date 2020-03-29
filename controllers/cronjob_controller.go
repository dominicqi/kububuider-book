/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/robfig/cron"
	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
	"time"

	batchv1 "kubebuilder/learn/api/v1"
)

var (
	jobOwnerKey             = ".metadata.controller"
	scheduledTimeAnnotation = "batch.tutorial.kubebuilder.io/scheduled-at"
)

// CronJobReconciler reconciles a CronJob object
type CronJobReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=batch.tutorial.kubebuilder.io,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch.tutorial.kubebuilder.io,resources=cronjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resource=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resource=jobs/status,verbs=get

func (r *CronJobReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("cronjob", req.NamespacedName)

	// your logic here
	cronjob := batchv1.CronJob{}
	if err := r.Get(ctx, req.NamespacedName, &cronjob); err != nil {
		log.Error(err, "unable to fetch CronJob")
		return ctrl.Result{}, err
	}
	var childJobs kbatch.JobList
	if err := r.List(ctx, &childJobs, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name}); err != nil {
		log.Error(err, "unable to list child Jobs")
		return ctrl.Result{}, err
	}

	var activeJobs []*kbatch.Job
	var successfulJobs []*kbatch.Job
	var failedJob []*kbatch.Job
	var mostRecentTime *time.Time

	/*
		判断job是否完成 以及完成类型
	*/
	isJobFinished := func(job *kbatch.Job) (bool, kbatch.JobConditionType) {
		for _, c := range job.Status.Conditions {
			if (c.Type == kbatch.JobComplete || c.Type == kbatch.JobFailed) && c.Status == corev1.ConditionTrue {
				return true, c.Type
			}
		}
		return false, ""
	}
	/*
		从job的annotation中取出创建的时候加入的调度时间
	*/
	getScheduledTimeForJob := func(job *kbatch.Job) (*time.Time, error) {
		timeRaw := job.Annotations[scheduledTimeAnnotation]
		if len(timeRaw) == 0 {
			return nil, nil
		}
		timeParsed, err := time.Parse(time.RFC3339, timeRaw)
		if err != nil {
			return nil, err
		}
		return &timeParsed, nil
	}

	for _, job := range childJobs.Items {
		_, conditionType := isJobFinished(&job)

		switch conditionType {
		case "":
			activeJobs = append(activeJobs, &job)
		case kbatch.JobFailed:
			failedJob = append(failedJob, &job)
		case kbatch.JobComplete:
			successfulJobs = append(successfulJobs, &job)
		}
		scheduledTimeForJob, err := getScheduledTimeForJob(&job)
		if err != nil {
			log.Error(err, "unable to parse schedule time from job annotation", "job", &job)
			continue
		}
		if scheduledTimeForJob != nil {
			if mostRecentTime == nil {
				mostRecentTime = scheduledTimeForJob
			} else if mostRecentTime.Before(*scheduledTimeForJob) {
				mostRecentTime = scheduledTimeForJob
			}
		}

		if mostRecentTime == nil {
			cronjob.Status.LiastScheduleTime = nil
		} else {
			cronjob.Status.LiastScheduleTime = &metav1.Time{Time: *mostRecentTime}
		}

		cronjob.Status.Active = nil

		for _, activeJob := range activeJobs {
			jobRef, err := reference.GetReference(r.Scheme, activeJob)
			if err != nil {
				log.Error(err, "unable to make a reference to active job", "job", activeJob)
				continue
			}
			cronjob.Status.Active = append(cronjob.Status.Active, *jobRef)
		}
		log.V(1).Info("job count", "active jobs", len(activeJobs), "successful jobs", len(successfulJobs), "failed jobs", len(failedJob))

		if err := r.Status().Update(ctx, &cronjob); err != nil {
			log.Error(err, "unable to update CronJob status")
			return ctrl.Result{}, err
		}

		deleteHistoryJob := func(jobs []*kbatch.Job, limit int, jobType string) {
			if len(jobs) > limit {
				sort.Slice(jobs, func(i, j int) bool {
					if jobs[i].Status.StartTime == nil {
						return jobs[j].Status.StartTime != nil
					}
					return jobs[i].Status.StartTime.Before(jobs[j].Status.StartTime)
				})
				for i := range jobs[0 : len(jobs)-limit] {
					if err := r.Delete(ctx, jobs[i]); err != nil {
						log.Error(err, "unable delete old  job", "jobType", jobType, "job", jobs[i])
					} else {
						log.V(0).Info("deleted old job", "jobType", jobType, "job", jobs[i])
					}
				}
			}
		}

		if cronjob.Spec.FailedJobHistoryLimit != nil {
			deleteHistoryJob(failedJob, int(*cronjob.Spec.FailedJobHistoryLimit), "failed")
		}

		if cronjob.Spec.SuccessfulJobHistoryLimit != nil {
			deleteHistoryJob(successfulJobs, int(*cronjob.Spec.FailedJobHistoryLimit), "successful")
		}

		if cronjob.Spec.Suspend != nil && *cronjob.Spec.Suspend {
			log.V(1).Info("cronjob suspended, skipping")
			return ctrl.Result{}, nil
		}

		getNextSchedule := func(cronJob batchv1.CronJob, now time.Time) (lastMissed time.Time, next time.Time, err error) {
			sched, err := cron.ParseStandard(cronjob.Spec.Schedule)
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("parse cron failed %q:%v", cronjob.Spec.Schedule, err)
			}

			var earliestTime time.Time
			if cronjob.Status.LiastScheduleTime != nil {
				earliestTime = cronjob.Status.LiastScheduleTime.Time
			} else {
				earliestTime = cronjob.CreationTimestamp.Time
			}

			if cronJob.Spec.StartingDeadlineSeconds != nil {
				//假设上一个过deadline的时间点
				schedulingDeadline := now.Add(-time.Second * time.Duration(*cronJob.Spec.StartingDeadlineSeconds))
				//这样计算出来的下一次调度时间一定没有过deadline
				if schedulingDeadline.After(earliestTime) {
					earliestTime = schedulingDeadline
				}
			}

			if earliestTime.After(now) {
				return time.Time{}, sched.Next(now), nil
			}
			starts := 0
			for t := sched.Next(earliestTime); !t.After(now); t = sched.Next(t) {
				lastMissed = t

				starts++

				if starts > 100 {
					return time.Time{}, time.Time{}, fmt.Errorf("Too many missed start times,check your deadline,or check clock skew.")
				}
			}
			return lastMissed, sched.Next(now), nil
		}

	}

	return ctrl.Result{}, nil
}

func (r *CronJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.CronJob{}).
		Complete(r)
}
