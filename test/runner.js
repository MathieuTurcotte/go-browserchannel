var page = require('webpage').create();
var system = require('system');

page.open('http://hpenvy.local:8080/', function(status) {
    if (status != 'success') {
        console.log('failed to open webpage');
    }
});

page.onCallback = function(data) {
    console.log(JSON.stringify(data));
    page.close();
    phantom.exit(data.success === true ? 0 : 1);
};

page.onConsoleMessage = function(msg, lineNum, sourceId) {
    system.stdout.write(msg);
};

setTimeout(function() {
    console.log('timeout');
    phantom.exit(1);
}, 10 * 10000);
