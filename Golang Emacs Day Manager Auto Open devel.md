Golang Emacs Day Manager Auto Open development plan:

Thing I want included into the project:
1. Run Emacs on daemon and single instance
    If Emacs daemon in running open emacs  [done]
    If Emacs is running open in the current [done]
    App opens the todays .org file and possibly yesterdays too for comparison

2. Enable automatic execution
    If a project is tagged "automatic open" <- can only be one
    Run the "daily workflow" automatically on windows start
    It should be marked with a heart icon and be at the very top of the list always
    
3. Prolly not open 2 windows if there is no previous file 