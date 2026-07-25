; Definitions
(function_declaration
  name: (identifier) @def.function.name) @def.function

(method_declaration
  name: (field_identifier) @def.method.name) @def.method

(type_spec
  name: (type_identifier) @def.type.name) @def.type

(type_alias
  name: (type_identifier) @def.type.name) @def.type

; References
(call_expression
  function: (identifier) @ref.call)

(call_expression
  function: (selector_expression) @ref.call)

(import_spec
  path: (interpreted_string_literal) @ref.import)
