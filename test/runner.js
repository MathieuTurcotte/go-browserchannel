/*
 * Copyright (c) 2013 Mathieu Turcotte
 * Licensed under the MIT license.
 */

var webpage = require('webpage');
var system = require('system');

var options = {
    port: 8080,
    host: 'hpenvy.local',
    tests: ['1', '2']
};

var args = system.args.slice(1);

while (args.length) {
    arg = args.shift();
    switch (arg) {
        case '--port':
            options.port = args.shift();
            break;
        case '--host':
            options.host = args.shift();
            break;
        case '--tests':
            options.tests = args.shift().split(',');
            break;
        default:
            console.error('unrecognized option: ' + arg)
            phantom.exit(1);
    }
}

console.log('Running with options: ' + JSON.stringify(options));

var tests = {};
var numTests = 0;

// Construct and lauch the test cases.
options.tests.forEach(function(id) {
    var host = options.host;
    var port = options.port;
    var url = 'http://' + host + ':' + port + '/?test=' + id;
    var page = webpage.create();

    console.info('Starting test ' + id + ' at ' + url);

    var test = {
        id: id,
        url: url,
        output: [],
        done: false,
        success: false
    };

    page.onCallback = function(msg) {
        switch (msg.type) {
            case 'done':
                done(test, msg.success);
                break;
            default:
                console.error('unknown msg type: ' + JSON.stringify(msg));
                done(test, false /* success */);
        }
    };

    page.onConsoleMessage = function(msg, lineNum, sourceId) {
        system.stdout.write('test ' + id + ': ' + msg);
        test.output.push(msg);
    };

    page.open(url, function(status) {
        if (status != 'success') {
            console.error('Failed to open test: ' + id);
            done(test, false /* success */);
        }
    });

    tests[id] = test;
    numTests++;
});

// Called when a test case is completed.
function done(test, success) {
    console.log('Test ' + test.id + ' completed.');

    test.done = true;
    test.success = success;

    numTests--;

    if (numTests == 0) {
        // Calling exit within the callback causes phantom to crash.
        setTimeout(complete, 1);
    }
}

// Called when a test cases are completed.
function complete() {
    var numPass = 0;
    var numFail = 0;

    for (var id in tests) {
        var test = tests[id];
        if (test.success) {
            numPass++;
        } else {
            numFail++;
        }
    }

    console.log('All tests completed.');
    console.log('Pass: ' + numPass);
    console.log('Fail: ' + numFail);

    phantom.exit(numFail > 0 ? 1 : 0);
}
