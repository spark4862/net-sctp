//go:generate mockgen -destination ../../mock/ -package mock -build_flags=-mod=readonly github.com/gorilla/websocket Conn

package sender

// 这块实在不好mock,我就直接依赖了
