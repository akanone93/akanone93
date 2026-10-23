// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim

import "net/http"

// supported is a feature flag of the service provider configuration.
type supported struct {
	Supported bool `json:"supported"`
}

// configDocument is the service provider configuration (RFC 7643 §5).
type configDocument struct {
	Schemas        []string         `json:"schemas"`
	Patch          supported        `json:"patch"`
	Bulk           bulkConfig       `json:"bulk"`
	Filter         filterConfig     `json:"filter"`
	ChangePassword supported        `json:"changePassword"`
	Sort           supported        `json:"sort"`
	ETag           supported        `json:"etag"`
	Authentication []authentication `json:"authenticationSchemes"`
	Meta           configMeta       `json:"meta"`
}

type bulkConfig struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

type filterConfig struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

type authentication struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SpecURI     string `json:"specUri"`
	Primary     bool   `json:"primary"`
}

type configMeta struct {
	ResourceType string `json:"resourceType"`
	Location     string `json:"location"`
}

// serviceProviderConfig serves GET /ServiceProviderConfig.
func (s *server) serviceProviderConfig(c call, _ string) {
	doc := configDocument{
		Schemas: []string{schemaConfig}, Patch: supported{true},
		Filter: filterConfig{Supported: true, MaxResults: s.dir.Limits().MaxResults}, ETag: supported{true},
		Authentication: []authentication{{
			Type: "oauthbearertoken", Name: "Bearer token", Primary: true,
			Description: "A SCIM token created for the organisation in Plux Studio, sent as Authorization: Bearer.",
			SpecURI:     "https://www.rfc-editor.org/info/rfc6750",
		}},
		Meta: configMeta{ResourceType: "ServiceProviderConfig", Location: mount + "/ServiceProviderConfig"},
	}
	s.respond(c, http.StatusOK, doc, 0, "")
}

// schemaAttribute is an attribute in a schema definition (RFC 7643 §7).
type schemaAttribute struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	MultiValued   bool              `json:"multiValued"`
	Description   string            `json:"description"`
	Required      bool              `json:"required"`
	CaseExact     bool              `json:"caseExact"`
	Mutability    string            `json:"mutability"`
	Returned      string            `json:"returned"`
	Uniqueness    string            `json:"uniqueness"`
	SubAttributes []schemaAttribute `json:"subAttributes,omitempty"`
}

// schemaDefinition is a schema as /Schemas returns it.
type schemaDefinition struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Attributes  []schemaAttribute `json:"attributes"`
	Meta        configMeta        `json:"meta"`
}

// attribute builds a schema attribute with the defaults of the attributes
// Plux keeps: read-write, always returned.
func attribute(name, kind, description string, required bool, uniqueness string) schemaAttribute {
	return schemaAttribute{
		Name: name, Type: kind, Description: description, Required: required,
		Mutability: "readWrite", Returned: "default", Uniqueness: uniqueness,
	}
}

// readOnly marks an attribute the server owns.
func readOnly(a schemaAttribute) schemaAttribute {
	a.Mutability = "readOnly"
	return a
}

// schemas returns the two schemas the server implements.
func schemas() []schemaDefinition {
	userMember := readOnly(attribute("value", "string", "The identifier of the group.", false, "none"))
	groupMembers := schemaAttribute{
		Name: "members", Type: "complex", MultiValued: true, Description: "The members of the group, by user identifier.",
		Mutability: "readWrite", Returned: "default", Uniqueness: "none",
		SubAttributes: []schemaAttribute{
			attribute("value", "string", "The identifier of the user.", true, "none"),
			readOnly(attribute("display", "string", "The user's name.", false, "none")),
			readOnly(attribute("$ref", "reference", "The location of the user.", false, "none")),
			readOnly(attribute("type", "string", "Always User.", false, "none")),
		},
	}
	groups := readOnly(schemaAttribute{
		Name: "groups", Type: "complex", MultiValued: true, Description: "The groups the user belongs to.",
		Returned: "default", Uniqueness: "none",
		SubAttributes: []schemaAttribute{userMember, readOnly(attribute("display", "string", "The group's name.", false, "none"))},
	})
	return []schemaDefinition{
		{
			ID: schemaUser, Name: "User", Description: "A person of the organisation.",
			Attributes: []schemaAttribute{
				attribute("userName", "string", "The person's email address, which identifies the account.", true, "server"),
				attribute("externalId", "string", "The identity provider's identifier of the person.", false, "none"),
				attribute("displayName", "string", "The person's name.", false, "none"),
				attribute("active", "boolean", "False once the person has left; their access ends at once.", false, "none"),
				groups,
			},
			Meta: configMeta{ResourceType: "Schema", Location: mount + "/Schemas/" + schemaUser},
		},
		{
			ID: schemaGroup, Name: "Group", Description: "A team of the organisation.",
			Attributes: []schemaAttribute{
				attribute("displayName", "string", "The name of the group.", true, "none"),
				attribute("externalId", "string", "The identity provider's identifier of the group.", false, "none"),
				groupMembers,
			},
			Meta: configMeta{ResourceType: "Schema", Location: mount + "/Schemas/" + schemaGroup},
		},
	}
}

// resourceType is a resource type as /ResourceTypes returns it (RFC 7643 §6).
type resourceType struct {
	Schemas     []string   `json:"schemas"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Endpoint    string     `json:"endpoint"`
	Description string     `json:"description"`
	Schema      string     `json:"schema"`
	Meta        configMeta `json:"meta"`
}

// resourceTypes returns the two resource types the server implements.
func resourceTypes() []resourceType {
	build := func(name, endpoint, description, schema string) resourceType {
		return resourceType{
			Schemas: []string{schemaResource}, ID: name, Name: name, Endpoint: endpoint, Description: description, Schema: schema,
			Meta: configMeta{ResourceType: "ResourceType", Location: mount + "/ResourceTypes/" + name},
		}
	}
	return []resourceType{
		build("User", "/Users", "A person of the organisation.", schemaUser),
		build("Group", "/Groups", "A team of the organisation.", schemaGroup),
	}
}

// listSchemas serves GET /Schemas.
func (s *server) listSchemas(c call, _ string) {
	all := schemas()
	s.respond(c, http.StatusOK, page(len(all), 1, all, len(all)), 0, "")
}

// getSchema serves GET /Schemas/{id}.
func (s *server) getSchema(c call, id string) {
	for _, d := range schemas() {
		if d.ID == id {
			s.respond(c, http.StatusOK, d, 0, "")
			return
		}
	}
	s.fail(c.w, c.r, reject(http.StatusNotFound, "", "there is no such schema"))
}

// listResourceTypes serves GET /ResourceTypes.
func (s *server) listResourceTypes(c call, _ string) {
	all := resourceTypes()
	s.respond(c, http.StatusOK, page(len(all), 1, all, len(all)), 0, "")
}

// getResourceType serves GET /ResourceTypes/{id}.
func (s *server) getResourceType(c call, id string) {
	for _, t := range resourceTypes() {
		if t.ID == id {
			s.respond(c, http.StatusOK, t, 0, "")
			return
		}
	}
	s.fail(c.w, c.r, reject(http.StatusNotFound, "", "there is no such resource type"))
}
