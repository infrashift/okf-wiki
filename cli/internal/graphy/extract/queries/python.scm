; Definitions
(function_definition
  name: (identifier) @def.function.name) @def.function

(class_definition
  name: (identifier) @def.class.name) @def.class

; References
(call
  function: (identifier) @ref.call)

(call
  function: (attribute) @ref.call)

(import_statement
  name: (dotted_name) @ref.import)

(import_statement
  name: (aliased_import
    name: (dotted_name) @ref.import))

(import_from_statement
  module_name: (dotted_name) @ref.import)

(class_definition
  superclasses: (argument_list
    (identifier) @ref.inherit))
