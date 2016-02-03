package worker_test

import (
	"testing"

	"github.com/WatchBeam/redutil/conn"
	"github.com/WatchBeam/redutil/queue"
	"github.com/WatchBeam/redutil/test"
	"github.com/WatchBeam/redutil/worker"
	"github.com/garyburd/redigo/redis"
	"github.com/stretchr/testify/suite"
)

type DefaultLifecycleSuite struct {
	*test.RedisSuite
}

func TestDefaultLifecycleSuite(t *testing.T) {
	pool, _ := conn.New(conn.ConnectionParam{
		Address: "127.0.0.1:6379",
	}, 1)

	suite.Run(t, &DefaultLifecycleSuite{test.NewSuite(pool)})
}

func (suite *DefaultLifecycleSuite) TestConstruction() {
	l := worker.NewLifecycle(suite.Pool, "queue", "worker_1")

	suite.Assert().IsType(&worker.DefaultLifecycle{}, l)
}

func (suite *DefaultLifecycleSuite) TestListenReturnsTasks() {
	l := worker.NewLifecycle(suite.Pool, "queue", "worker_1")
	queue := queue.NewByteQueue(suite.Pool, "queue")
	tasks, _ := l.Listen()
	defer func() {
		l.AbandonAll()
	}()

	queue.Push([]byte("hello, world!"))

	task := <-tasks
	l.StopListening()

	suite.Assert().Equal([]byte("hello, world!"), task.Bytes())
}

func (suite *DefaultLifecycleSuite) TestCompletedTasksRemovedFromAllQueues() {
	l := worker.NewLifecycle(suite.Pool, "queue", "worker_1")
	defer l.StopListening()

	queue := queue.NewByteQueue(suite.Pool, "queue")
	queue.Push([]byte("some_task"))

	tasks, _ := l.Listen()

	l.Complete(<-tasks)

	suite.WithRedis(func(conn redis.Conn) {
		ql := suite.RedisLength("queue")
		wl := suite.RedisLength("queue:_worker_worker_1")

		suite.Assert().Equal(0, ql, "redutil: main queue should be empty, but wasn't")
		suite.Assert().Equal(0, wl, "redutil: worker (worker_1) queue should be empty, but wasn't")
	})
}

func (suite *DefaultLifecycleSuite) TestAbandonedTasksRemovedFromWorkerQueue() {
	cnx := suite.Pool.Get()
	defer cnx.Close()

	l := worker.NewLifecycle(suite.Pool, "queue", "worker_1")

	queue := queue.NewByteQueue(suite.Pool, "queue")
	queue.Push([]byte("some_task"))

	tasks, _ := l.Listen()
	task := <-tasks
	l.StopListening()

	suite.Assert().Equal(1, suite.RedisLength("queue:worker_worker_1"),
		"redutil: worker (worker_1) queue should have one item, but doesn't")
	suite.Assert().Equal(0, suite.RedisLength("queue"),
		"redutil: main queue should be empty, but wasn't")

	l.Abandon(task)

	suite.Assert().Equal(0, suite.RedisLength("queue:worker_worker_1"),
		"redutil: worker (worker_1) should be empty, but isn't")
	suite.Assert().Equal(1, suite.RedisLength("queue"),
		"redutil: main queue should have one item, but doesn't")
}

func (suite *DefaultLifecycleSuite) TestAbandonAllMovesAllTasksToMainQueue() {
	l := worker.NewLifecycle(suite.Pool, "queue", "worker_1")

	queue := queue.NewByteQueue(suite.Pool, "queue")
	queue.Push([]byte("task_1"))
	queue.Push([]byte("task_2"))
	queue.Push([]byte("task_3"))

	tasks, _ := l.Listen()
	for i := 0; i < 3; i++ {
		<-tasks
	}
	l.StopListening()

	suite.Assert().Equal(3, suite.RedisLength("queue:worker_worker_1"))
	suite.Assert().Equal(0, suite.RedisLength("queue"))

	l.AbandonAll()

	suite.Assert().Equal(0, suite.RedisLength("queue:worker_worker_1"))
	suite.Assert().Equal(3, suite.RedisLength("queue"))
}

func (suite *DefaultLifecycleSuite) RedisLength(keyspace string) int {
	cnx := suite.Pool.Get()
	defer cnx.Close()

	len, err := redis.Int(cnx.Do("LLEN", keyspace))
	if err != nil {
		return -1
	}

	return len
}
