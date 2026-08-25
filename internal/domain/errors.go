package domain

import "errors"

var (
	ErrNotFound           = errors.New("资源不存在")
	ErrConflict           = errors.New("版本冲突")
	ErrInvalid            = errors.New("输入不合法")
	ErrForbidden          = errors.New("角色无权执行此操作")
	ErrFrozen             = errors.New("作业已经冻结，不允许修改")
	ErrInvalidTransition  = errors.New("状态迁移不合法")
	ErrOpenSevereFinding  = errors.New("仍有未关闭的严重发现")
	ErrIncompleteCoverage = errors.New("载体尚未全部被有效采集覆盖")
	ErrDuplicateCapture   = errors.New("采集内容摘要与其他有效采集重复")
	ErrFilenameCollision  = errors.New("母版文件名与其他有效采集碰撞")
	ErrLineageConflict    = errors.New("采集版本谱系冲突")
	ErrStalePreview       = errors.New("冻结预检已经过期，请重新预检")
	ErrManifestBlocked    = errors.New("冻结预检仍有阻断项")
)

type ValidationError struct {
	Field, Message string
	Cause          error
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }
func (e *ValidationError) Unwrap() error { return e.Cause }

func Invalid(field, message string) error { return &ValidationError{Field: field, Message: message} }
func InvalidBecause(field, message string, cause error) error {
	return &ValidationError{Field: field, Message: message, Cause: cause}
}
