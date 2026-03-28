--border
make selected app border wider and better visible which application is currently selected
your fix with border is not working for ttyd application also make sure you will use the access color of the application for this border
looks like border selection is broken i can see borders around a 2 ttyd application and then around none
use frontend-design to improve the "selected app" look beautiful

--icon
improve discovering icons of the ttyd application. opencode icon is not working use web fetch to search for given icon if not found locally. store found icons in database

--web
when user use web-browser component to access some web url and this url exists store this url into database for later usage. next when user will open search popup and show and fuzzy-search also along this stored url addressed

--version
display version of the application next to the shortcut buttton

--port (done)
create prod/main.go with prod version of this project. dev should be started on port 1439 prod on port 8100. make sure install will use production version and air (i'm running this project for develeopement) will use dev version

--chrome
