package user

const (
	RoleUserA = "USER_A"
	RoleUserB = "USER_B"
	RoleOS    = "OS"
	RoleAdmin = "ADMIN"
)

type User struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Active bool   `json:"-"`
}
