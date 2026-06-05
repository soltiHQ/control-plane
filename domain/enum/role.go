package enum

// BuiltinRole describes a well-known role seeded at startup.
type BuiltinRole struct {
	ID          string
	Name        string
	Permissions []Permission
}

// Built-in role IDs.
//
// Numbering scheme: XYZ
//   - X: reserved (0)
//   - Y: group (0 = global, 1 = users, 2 = specs, 3 = agents)
//   - Z: level (1 = admin, 2 = editor, 3 = reader)
const (
	RoleAdminID  = "001"
	RoleEditorID = "002"
	RoleReaderID = "003"

	RoleUserAdminID  = "011"
	RoleUserEditorID = "012"
	RoleUserReaderID = "013"

	RoleSpecAdminID  = "021"
	RoleSpecEditorID = "022"
	RoleSpecReaderID = "023"

	RoleAgentEditorID = "031"
	RoleAgentReaderID = "032"
)

// BuiltinRoles contains all roles the control-plane seeds on the first startup.
var BuiltinRoles = []BuiltinRole{
	// Global
	{RoleAdminID, "Admin", All},
	{RoleEditorID, "Editor", []Permission{
		AgentsGet, AgentsEdit,
		UsersGet, UsersAdd, UsersEdit,
		SpecsGet, SpecsAdd, SpecsEdit, SpecsDeploy,
	}},
	{RoleReaderID, "Reader", []Permission{AgentsGet, UsersGet, SpecsGet}},

	// Users
	{RoleUserAdminID, "userAdmin", []Permission{UsersGet, UsersAdd, UsersEdit, UsersDelete}},
	{RoleUserEditorID, "userEditor", []Permission{UsersGet, UsersAdd, UsersEdit}},
	{RoleUserReaderID, "userReader", []Permission{UsersGet}},

	// Specs
	{RoleSpecAdminID, "specAdmin", []Permission{SpecsGet, SpecsAdd, SpecsEdit, SpecsDeploy, SpecsDelete}},
	{RoleSpecEditorID, "specEditor", []Permission{SpecsGet, SpecsAdd, SpecsEdit, SpecsDeploy}},
	{RoleSpecReaderID, "specReader", []Permission{SpecsGet}},

	// Agents
	{RoleAgentEditorID, "agentEditor", []Permission{AgentsGet, AgentsEdit}},
	{RoleAgentReaderID, "agentReader", []Permission{AgentsGet}},
}
