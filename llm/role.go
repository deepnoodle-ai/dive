package llm

type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	System    Role = "system"
)

func (r Role) String() string {
	return string(r)
}
