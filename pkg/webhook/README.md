# Validation Rules

The admission webhook validates ApplicationConfiguration spec according to the following rules. 


- RevisionName & ComponentName of component MUST be mutually exclusive. It's not allowed to assign both but one of them must be assigned.
- If a component is versioning enabled (that means its revisionName is assigned or it contains any revisionEnabled trait), its workload `metadata.name` MUST NOT be assigned value nor overwritten by parameters.

