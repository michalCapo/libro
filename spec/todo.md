--icon (todo)
improve discovering icons of the ttyd application. opencode icon is not working use web fetch to search for given icon if not found locally. store found icons in database

--web
when user use web-browser component to access some web url and this url exists store this url into database for later usage. next when user will open search popup and show and fuzzy-search also along this stored url addressed

--chrome (done)
make sure user can run multiple browsers applications inside one project, now when user launch more then one all of them freeze until user leaves only one open

--ttyd (done)
something is wrong with ttyd applications, i click on vim but it will start claude app. i have defined vim and bash ttyd applications. when i open vim first time it will start claude application, second time it vill start neovim, third time it will start vim. whne i click on bash appllication it will start claude, second time it will start bash as expected. is there problem with havining running mutliple ttyd at once? it shere problem with database or saving?

--project
simplyfy the new project popoup remove name field from form and use just directory name for this. also make the popup bigger and expand the count of visible folder for better scrolling

--browser
implement ctrl+l to focus and select url address so user can change it, also implement ctrl+r to reload the web content of the URL
