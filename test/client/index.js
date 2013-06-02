/*
 * Copyright (c) 2013 Mathieu Turcotte
 * Licensed under the MIT license.
 */

goog.provide('tests.start');

goog.require('goog.Uri');
goog.require('goog.debug.Console');
goog.require('goog.net.BrowserChannel');
goog.require('goog.net.ChannelDebug');
goog.require('tests.Handler1');
goog.require('tests.Handler2');


/**
 * Test driver entry point. Selects a test handler based on the "test" query
 * parameter and connects the browser channel configured with the handler to
 * the server.
 */
tests.start = function() {
    goog.debug.Console.autoInstall();
    goog.debug.Console.instance.setCapturing(true);
    var logger = goog.debug.Logger.getLogger('tests');

    var uri = new goog.Uri(window.location);
    var id = uri.getParameterValue('test');

    var testHandlers = {
        1: new tests.Handler1(),
        2: new tests.Handler2()
    };

    var handler = testHandlers[id];

    if (!handler) {
        logger.severe('no test registered for "' + id + '".');
        window.callPhantom({type: 'done', success: false});
        return;
    }

    var clientVersion = 'cv1';
    var channel = new goog.net.BrowserChannel(clientVersion);
    channel.setChannelDebug(new goog.net.ChannelDebug());
    channel.setSupportsCrossDomainXhrs(true);
    channel.setAllowHostPrefix(true);
    channel.setHandler(handler);
    channel.connect('channel/test', 'channel/bind');
};

goog.exportSymbol('tests.start', tests.start);
