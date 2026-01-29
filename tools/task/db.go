// tools/task/db.go
package task

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Project struct {
	ID        int64
	UserID    int64
	Name      string
	CreatedAt time.Time
}

type Task struct {
	ID        int64
	ProjectID int64
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TaskDB struct {
	db *sql.DB
}

func NewTaskDB(dbPath string) (*TaskDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}

	taskDB := &TaskDB{db: db}
	if err := taskDB.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return taskDB, nil
}

func (t *TaskDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, name)
	);

	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY,
		project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := t.db.Exec(schema)
	return err
}

func (t *TaskDB) Close() error {
	return t.db.Close()
}

func (t *TaskDB) CreateProject(userID int64, name string) (*Project, error) {
	result, err := t.db.Exec(
		"INSERT INTO projects (user_id, name) VALUES (?, ?)",
		userID, name,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Project{ID: id, UserID: userID, Name: name, CreatedAt: time.Now()}, nil
}

func (t *TaskDB) GetProject(userID int64, name string) (*Project, error) {
	var p Project
	err := t.db.QueryRow(
		"SELECT id, user_id, name, created_at FROM projects WHERE user_id = ? AND name = ?",
		userID, name,
	).Scan(&p.ID, &p.UserID, &p.Name, &p.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &p, err
}

func (t *TaskDB) GetOrCreateProject(userID int64, name string) (*Project, error) {
	p, err := t.GetProject(userID, name)
	if err == nil {
		return p, nil
	}
	if err != ErrNotFound {
		return nil, err
	}
	return t.CreateProject(userID, name)
}

func (t *TaskDB) CreateTask(projectID int64, name string) (*Task, error) {
	result, err := t.db.Exec(
		"INSERT INTO tasks (project_id, name) VALUES (?, ?)",
		projectID, name,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Task{
		ID:        id,
		ProjectID: projectID,
		Name:      name,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (t *TaskDB) GetTask(id int64) (*Task, error) {
	var task Task
	err := t.db.QueryRow(
		"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE id = ?",
		id,
	).Scan(&task.ID, &task.ProjectID, &task.Name, &task.Status, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &task, err
}

func (t *TaskDB) ListTasks(projectID int64, status string) ([]Task, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = t.db.Query(
			"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE project_id = ? ORDER BY created_at",
			projectID,
		)
	} else {
		rows, err = t.db.Query(
			"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE project_id = ? AND status = ? ORDER BY created_at",
			projectID, status,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.ProjectID, &task.Name, &task.Status, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (t *TaskDB) UpdateTaskStatus(id int64, status string) error {
	_, err := t.db.Exec(
		"UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
		status, time.Now(), id,
	)
	return err
}

func (t *TaskDB) DeleteTask(id int64) error {
	_, err := t.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

func (t *TaskDB) FindTaskByName(projectID int64, name string) (*Task, error) {
	var task Task
	err := t.db.QueryRow(
		"SELECT id, project_id, name, status, created_at, updated_at FROM tasks WHERE project_id = ? AND name = ?",
		projectID, name,
	).Scan(&task.ID, &task.ProjectID, &task.Name, &task.Status, &task.CreatedAt, &task.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &task, err
}
