; Definitions
(function_declaration
  name: (identifier) @def.function.name) @def.function

(generator_function_declaration
  name: (identifier) @def.function.name) @def.function

(class_declaration
  name: (type_identifier) @def.class.name) @def.class

(method_definition
  name: (property_identifier) @def.method.name) @def.method

(variable_declarator
  name: (identifier) @def.function.name
  value: (arrow_function)) @def.function

(variable_declarator
  name: (identifier) @def.function.name
  value: (function_expression)) @def.function

(interface_declaration
  name: (type_identifier) @def.interface.name) @def.interface

(type_alias_declaration
  name: (type_identifier) @def.type.name) @def.type

(enum_declaration
  name: (identifier) @def.type.name) @def.type

; References
(call_expression
  function: (identifier) @ref.call)

(call_expression
  function: (member_expression) @ref.call)

(import_statement
  source: (string) @ref.import)

(extends_clause
  value: (identifier) @ref.inherit)

(implements_clause
  (type_identifier) @ref.implement)
