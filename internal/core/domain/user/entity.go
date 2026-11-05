package user

const (
	RoleUserA = "USER_A"
	RoleUserB = "USER_B"
	RoleOS    = "OS"
	RoleAdmin = "ADMIN"
)

type User struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Email     string `json:"email"`
	StudentID string `json:"-"`
	Role      string `json:"role"`
	Active    bool   `json:"-"`
}
