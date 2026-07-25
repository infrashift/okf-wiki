; Definitions
(function_declaration
  name: (identifier) @def.function.name) @def.function

(generator_function_declaration
  name: (identifier) @def.function.name) @def.function

(class_declaration
  name: (identifier) @def.class.name) @def.class

(method_definition
  name: (property_identifier) @def.method.name) @def.method

(variable_declarator
  name: (identifier) @def.function.name
  value: (arrow_function)) @def.function

(variable_declarator
  name: (identifier) @def.function.name
  value: (function_expression)) @def.function

; References
(call_expression
  function: (identifier) @ref.call)

(call_expression
  function: (member_expression) @ref.call)

(import_statement
  source: (string) @ref.import)

(class_declaration
  (class_heritage
    (identifier) @ref.inherit))
