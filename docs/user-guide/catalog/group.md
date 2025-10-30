# Group

> An organizational entity (team or business unit) used to model ownership and
> contact information.

The `spec` of a `Group` entity has the following fields:

* `type` - *required* - The type of group (e.g., "team", "business-unit").
* `profile` - *optional* - Profile information about the group.
    * `displayName` - *optional* - A display name for the group.
    * `email` - *optional* - An email for the group.
    * `picture` - *optional* - A URL for a picture of the group.
* `members` - *optional* - A list of members of the group.

Example:

```yaml
apiVersion: swcat/v1
kind: Group
metadata:
    name: my-team
spec:
  type: team
  profile:
    displayName: My Team
    email: my-team@example.com
  parent: parent-group
  members:
    - John Doe
```
