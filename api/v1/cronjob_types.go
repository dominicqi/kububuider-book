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

package v1

import (
	"k8s.io/api/batch/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CronJobSpec defines the desired state of CronJob
type CronJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:MinLength=0

	//Cron表达式
	Schedule string `json:"schedule"`

	//job 无论任何原因错过语气执行时间的截止日期(秒) ，错误过的任务统计为失败的。
	// +optional
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`

	//并发处理行为
	// +optional
	ConcurrentPolicy ConcurrentPolicy `json:"concurrentPolicy,omitempty"`

	//暂停标识 默认false
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	//任务模板
	JobTemplate v1beta1.JobTemplateSpec `json:"jobTemplate"`

	// 成功记录保留最大条数
	// +kubebuilder:validation:
	SuccessfulJobHistoryLimit *int32 `json:"successfulJobHistoryLimit,omitempty"`

	// 失败记录保存最大条数
	// +kubebuilder:validation:Minimum=0
	FailedJobHistoryLimit *int32 `json:"failedJobHistoryLimit,omitempty"`
}

type ConcurrentPolicy string

const (
	// 允许Job并发执行
	AllowConcurrent ConcurrentPolicy = "Allow"

	//前一个还没有执行完的时候 跳过下一个
	ForbidConcurrent ConcurrentPolicy = "Forbid"

	// 停止当前的任务 替换成新的
	ReplaceConcurrent ConcurrentPolicy = "Replace"
)

// CronJobStatus defines the observed state of CronJob
type CronJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +optional
	Active []v1.ObjectReference `json:"active,omitempty"`

	// +optional
	LiastScheduleTime *metav1.Time `json:"liastScheduleTime,omitempty"`
}

// +kubebuilder:object:root=true

// CronJob is the Schema for the cronjobs API
type CronJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CronJobSpec   `json:"spec,omitempty"`
	Status CronJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CronJobList contains a list of CronJob
type CronJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CronJob{}, &CronJobList{})
}
