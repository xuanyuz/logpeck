package logpeck

import (
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/hpcloud/tail"
	"sync"
	"time"
)

type Pecker struct {
	logTasks   map[string]*LogTask
	nameToPath map[string]string
	db         *DB

	mu   sync.Mutex
	stop bool
}

func NewPecker(db *DB) (*Pecker, error) {
	pecker := &Pecker{
		logTasks:   make(map[string]*LogTask),
		nameToPath: make(map[string]string),
		db:         db,
		stop:       true,
	}
	err := pecker.restorePeckTasks(db)
	if err != nil {
		return nil, err
	}
	return pecker, nil
}

func (p *Pecker) restorePeckTasks(db *DB) error {
	defer LogExecTime(time.Now(), "Restore PeckTaskConfig")
	configs, err := p.db.GetAllConfigs()
	if err != nil {
		return err
	}
	for i, config := range configs {
		stat, _ := p.db.GetStat(config.Name)
		p.AddPeckTask(&config, stat)
		log.Infof("[Pecker] Restore PeckTask[%d] : %s", i, config)
	}
	return nil
}

// allow only modification of db/logTasks/nameToPath in this function
func (p *Pecker) record(config *PeckTaskConfig, stat *PeckTaskStat) {
	if _, ok := p.nameToPath[config.Name]; !ok {
		if _, ok2 := p.logTasks[config.LogPath]; !ok2 {
			p.logTasks[config.LogPath] = NewLogTask(config.LogPath)
		}
		p.nameToPath[config.Name] = config.LogPath
	}
	db.SaveConfig(config)
	if stat != nil {
		db.SaveStat(stat)
	}
}

func (p *Pecker) AddPeckTask(config *PeckTaskConfig, stat *PeckTaskStat) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	log.Infof("[Pecker] AddPeckTask %s", *config)
	if _, ok := p.nameToPath[config.Name]; ok {
		return errors.New("Peck task already exist")
	}

	task, err := NewPeckTask(config, stat)
	if err != nil {
		return err
	}

	p.record(config, &task.Stat)

	// AddPeckTask must be successful
	p.logTasks[p.nameToPath[config.Name]].AddPeckTask(task)

	log.Infof("[Pecker] Add PeckTask nameToPath: %v", p.nameToPath)
	log.Infof("[Pecker] Add PeckTask logTasks: %v", p.logTasks)
	return nil
}

func (p *Pecker) UpdatePeckTask(config *PeckTaskConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	log.Infof("[Pecker] UpdatePeckTask %s", *config)
	if _, ok := p.nameToPath[config.Name]; !ok {
		return errors.New("Peck task name not exist")
	}

	stat, err := db.GetStat(config.Name)
	task, err := NewPeckTask(config, stat)
	if err != nil {
		return err
	}

	p.record(config, &task.Stat)

	// UpdatePeckTask must be successful
	if err := p.logTasks[p.nameToPath[config.Name]].UpdatePeckTask(task); err != nil {
		return err
	}
	log.Infof("[Pecker] Update PeckTask nameToPath: %v", p.nameToPath)
	log.Infof("[Pecker] Update PeckTask logTasks: %v", p.logTasks)
	return nil
}

func (p *Pecker) RemovePeckTask(config *PeckTaskConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.nameToPath[config.Name]; !ok {
		return errors.New("Peck task name not exist")
	}

	log_path, ok1 := p.nameToPath[config.Name]
	log_task, ok2 := p.logTasks[log_path]
	if !ok1 || !ok2 {
		log.Panicf("%v\n%v\n%v", config.Name, p.nameToPath, p.logTasks)
	}

	log.Infof("[Pecker] Remove PeckTask try clean db: %s", config)
	err1 := db.RemoveConfig(config.Name)
	err2 := db.RemoveStat(config.Name)
	if err1 != nil || err2 != nil {
		panic(err1.Error() + " " + err2.Error())
	}

	if err := log_task.RemovePeckTask(config); err != nil {
		return err
	}
	delete(p.nameToPath, config.Name)
	if log_task.Empty() {
		log_task.Close()
		delete(p.logTasks, log_path)
	}
	log.Infof("[Pecker] Remove PeckTask nameToPath: %v", p.nameToPath)
	log.Infof("[Pecker] Remove PeckTask logTasks: %v", p.logTasks)
	return nil
}

func (p *Pecker) ListPeckTask() ([]PeckTaskConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	configs, err := p.db.GetAllConfigs()
	if err != nil {
		return nil, err
	}
	return configs, nil
}
func (p *Pecker) ListTaskStats() ([]PeckTaskStat, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stats, err := p.db.GetAllStats()
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (p *Pecker) StartPeckTask(config *PeckTaskConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	log_path, ok := p.nameToPath[config.Name]
	if !ok {
		log.Infof("Task not exist, Name: %s, Exist: %v", config.Name, p.nameToPath)
		return fmt.Errorf("Task not exist, Name: %s, Exist: %v", config.Name, p.nameToPath)
	}

	log_task := p.logTasks[log_path]

	if err := log_task.StartPeckTask(config); err != nil {
		return err
	}

	{
		// Try update peck task stat in boltdb
		// stat, err := db.GetStat(config.LogPath, config.Name)
		stat, err := db.GetStat(config.Name)
		if err != nil {
			return err
		}
		if !stat.Stop {
			return errors.New("Task already started")
		}
		stat.Stop = false
		err = db.SaveStat(stat)
	}
	if log_task.IsStop() {
		log_task.Start()
	}
	return nil
}

func (p *Pecker) StopPeckTask(config *PeckTaskConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	log.Infof("[Pecker]Try stop task, Name: %s, Exist: %v", config.Name, p.nameToPath)
	log_path, ok := p.nameToPath[config.Name]
	if !ok {
		log.Infof("Task not exist, Name: %s, Exist: %v", config.Name, p.nameToPath)
		return fmt.Errorf("Task not exist, Name: %s, Exist: %v", config.Name, p.nameToPath)
	}

	log_task := p.logTasks[log_path]

	if err := log_task.StopPeckTask(config); err != nil {
		return err
	}

	{
		// Try update peck task stat in boltdb
		stat, err := db.GetStat(config.Name)
		if err != nil {
			return err
		}
		if stat.Stop {
			return errors.New("Task already stopped")
		}
		stat.Stop = true
		err = db.SaveStat(stat)
	}

	return nil
}

func TestPeckTask(config *PeckTaskConfig) ([]map[string]interface{}, error) {
	task, err := NewPeckTask(config, nil)
	if err != nil {
		return []map[string]interface{}{}, err
	}
	tailConf := tail.Config{
		MustExist: true,
		ReOpen:    true,
		Poll:      true,
		Follow:    true,
		Location: &tail.SeekInfo{
			Offset: 0,
			Whence: 2,
		},
	}
	ch := make(chan bool, 1)
	resultsCh := make(chan map[string]interface{}, config.Test.TestNum)
	id := 0
	close := false
	tail, err := tail.TailFile(config.LogPath, tailConf)
	if err != nil {
		return []map[string]interface{}{}, err
	}
	go func() {
		for content := range tail.Lines {
			if close == true {
				break
			}
			fields, err := task.ProcessTest(content.Text)
			Log := make(map[string]interface{})
			if err != nil {
				if err.Error() == "Discarded" {
					continue
				}
				Log["_Error"] = err.Error()
				Log["_Log"] = content.Text
			} else if _, ok := fields["_Log"]; !ok {
				Log["_Log"] = content.Text
				Log["_Fields"] = fields
			} else {
				Log = fields
			}
			resultsCh <- Log
			id++
			if id >= config.Test.TestNum {
				break
			}
		}
		ch <- true
	}()
	var res []map[string]interface{}
	select {
	case <-ch:
	case <-time.After(time.Second * time.Duration(config.Test.Timeout)):
		close = true
	}
	l := len(resultsCh)
	for i := 0; i < l; i++ {
		res = append(res, <-resultsCh)
	}
	return res, nil
}

func (p *Pecker) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stop {
		return errors.New("Pecker already started")
	}
	for path, logTask := range p.logTasks {
		log.Infof("[Pecker] Start LogTask %s", path)
		logTask.Start()
	}
	return nil
}

func (p *Pecker) GetStat() *PeckerStat {
	p.mu.Lock()
	defer p.mu.Unlock()
	return nil
}
