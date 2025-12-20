Golang Emacs Day Manager Auto Open development plan:

Thing I want included into the project:
1. Run Emacs on daemon and single instance
    If Emacs daemon in running open emacs  [done]
    If Emacs is running open in the current [done]
    App opens the todays .org file and possibly yesterdays too for comparison [done]

    This overall has 1 bug which is not a big deal. If you close emacs then open a new one it like to go crazy
    For my purposes it's not worth fixing

2. Enable automatic execution
    If a project is tagged "automatic open" <- can only be one
    Run the "daily workflow" automatically on windows start
    It should be marked with a heart icon and be at the very top of the list always
    
3. Prolly not open 2 windows if there is no previous file 