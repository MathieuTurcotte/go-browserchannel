var page = require('webpage').create();
var system = require('system');

page.open('http://hpenvy.local:8080/', function(status) {
    if (status != 'success') {
        console.log('failed to open webpage');
    }
});

page.onCallback = function(data) {
    console.log(JSON.stringify(data));

    // Calling phantom.exit within the callback causes a crash.
    var success = data.success === true;
    setTimeout(function() {
        phantom.exit(success ? 0 : 1);
    }, 1);
};

page.onConsoleMessage = function(msg, lineNum, sourceId) {
    system.stdout.write(msg);
};
