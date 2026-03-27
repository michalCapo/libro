--ttyd (done)
Add support to use any terminal base application via ttyd (use https://github.com/tsl0922/ttyd)
make sure when user define application it will start on new port with -p parameter and make it editable via --writtable parameter

--close (done)
add close button to close application/iframe

--keyboard (done)
add keyboard shortcut down switch left/right the applications wint window/meta+h (for left) and window/meta+l to right. this will keep selected aplication in center

--project (done)
add project support. project is related to folder. Default project name is home which meanst it will start in home folder. User can create new project, he needs to define a folder where this project is related to. User can see list of project. When user clicks on project working directory will change to this folder. Now every application expectially ttyd applications can user this folder. Now all ttyd application will start with command 'cd <pwd>' and then appclition which will ensure every app starts in pwd folder (this will be done automatically)
When user switch form one project to another application will be kept live. App will remmembered position and size of application. When user switch project application from prev project will hide and current applications will become visible or will be crated if this is first time the project will be open in this session.

--picker (done)
when user adds new project use directory picker, determine name from this folder path, but add allow user to change it (name) after he pick the directory

--switch
when user click inside ttyd application shortcuts for switch prev/next application (win+h/l) shortcut ist not working. this is crutial for correct application functionality fix it

--open (done)
one application is already running. user open second application, but first application will reset. this is true for ttyd applications. make it work so already created application wont reset and will stay in current state. this same problem is when user close the second application then first will reset also. make sure running application will not reset until user close it.

--position (done)
when one application is open in project, center it horizontally. when 2 and more applications are opened in project start from left side.

--empty
add new button to application list which will open new window with default size and ask user about URL he wants to go to. this will work only for URL application. something like simple browser. if URL is not available create google.com search query and display it to user (iframe google with search parameter). This is quick browsing feature.
