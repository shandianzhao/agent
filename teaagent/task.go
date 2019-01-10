package teaagent

import (
	"errors"
	"github.com/TeaWeb/code/teaconfigs/agents"
	"github.com/iwind/TeaGo/timers"
	"log"
	"sync"
	"time"
)

// 任务定义
type Task struct {
	config        *agents.TaskConfig
	processes     []*Process
	processLocker sync.Mutex
	lastTimer     *time.Timer
}

// 获取新任务
func NewTask(config *agents.TaskConfig) *Task {
	return &Task{
		config: config,
	}
}

// 是否应该启动
func (this *Task) ShouldBoot() bool {
	return this.config.IsBooting
}

// 立即运行
func (this *Task) Run() (proc *Process, stdout string, stderr string, err error) {
	log.Println("run task", this.config.Id, this.config.Name)
	if this.config == nil {
		err = errors.New("task config should not be nil")
		return
	}

	// shell
	if len(this.config.Id) == 0 {
		err = errors.New("id should not be empty")
		return
	}

	var shFile string
	shFile, err = this.config.Generate()
	if err != nil {
		return
	}

	// execute
	proc = NewProcess()
	proc.Cwd = this.config.Cwd
	proc.Env = this.config.Env
	proc.File = shFile
	proc.OnStart(func() {
		this.processLocker.Lock()
		defer this.processLocker.Unlock()
		this.processes = append(this.processes, proc)
	})
	proc.OnStop(func() {
		this.processLocker.Lock()
		defer this.processLocker.Unlock()
		result := []*Process{}
		for _, p := range this.processes {
			if p == proc {
				continue
			}
			result = append(result, p)
		}
		this.processes = result
	})
	stdout, stderr, err = proc.Run()
	return
}

// 定时运行
func (this *Task) Schedule(fromTimer ... bool) {
	now := time.Now()

	// 第一次是否运行
	if len(fromTimer) == 0 {
		next, ok := this.config.Next(now)
		if !ok {
			return
		}
		if now.Unix() == next.Unix() {
			if !this.IsRunning() {
				go this.Run()
			}
			timers.Delay(1*time.Second, func(timer *time.Timer) {
				this.Schedule(true)
			})
			return
		}
	}

	now = now.Add(1 * time.Second)
	next, ok := this.config.Next(now)
	if !ok {
		return
	}
	this.lastTimer = timers.At(next, func(timer *time.Timer) {
		this.lastTimer = nil

		if !this.IsRunning() {
			go this.Run()
		}
		this.Schedule(true)
	})
}

// 是否正在运行
func (this *Task) IsRunning() bool {
	this.processLocker.Lock()
	defer this.processLocker.Unlock()

	return len(this.processes) > 0
}

// 停止
func (this *Task) Stop() error {
	this.processLocker.Lock()
	defer this.processLocker.Unlock()

	var resultError error = nil

	for _, p := range this.processes {
		err := p.Kill()
		if err != nil {
			resultError = err
		}
	}
	this.processes = []*Process{}

	if this.lastTimer != nil {
		this.lastTimer.Stop()
		this.lastTimer = nil
	}

	return resultError
}